package k8s

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	coordv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Tunable Lease parameters. LeaseDurationSeconds tells consumers
// (the operator) how long they should consider this relay alive
// without a renewal; renewInterval is how often we refresh it.
const (
	leaseDurationSeconds = 30
	renewInterval        = 10 * time.Second
)

// realClient is the in-cluster implementation of Client.
type realClient struct {
	logger    *slog.Logger
	typed     kubernetes.Interface
	dynamic   dynamic.Interface
	podName   string
	namespace string
}

func (c *realClient) PodName() string   { return c.podName }
func (c *realClient) Namespace() string { return c.namespace }

// Start ensures the Lease exists and renews it on a fixed cadence
// until ctx is cancelled. The first renewal is performed synchronously
// so the relay only reports Ready after the operator can see it.
func (c *realClient) Start(ctx context.Context) error {
	if err := c.renewLease(ctx); err != nil {
		return fmt.Errorf("initial lease renewal: %w", err)
	}
	go c.renewLoop(ctx)
	return nil
}

func (c *realClient) renewLoop(ctx context.Context) {
	t := time.NewTicker(renewInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := c.renewLease(ctx); err != nil {
				c.logger.Warn("lease renewal failed", "err", err.Error())
			}
		}
	}
}

// renewLease creates the Lease on first call and Updates it on
// subsequent calls. The Lease's holder identity is the pod name.
func (c *realClient) renewLease(ctx context.Context) error {
	leases := c.typed.CoordinationV1().Leases(c.namespace)
	now := metav1.NewMicroTime(time.Now())
	holder := c.podName
	dur := int32(leaseDurationSeconds)

	existing, err := leases.Get(ctx, c.podName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		_, err := leases.Create(ctx, &coordv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      c.podName,
				Namespace: c.namespace,
			},
			Spec: coordv1.LeaseSpec{
				HolderIdentity:       &holder,
				LeaseDurationSeconds: &dur,
				AcquireTime:          &now,
				RenewTime:            &now,
			},
		}, metav1.CreateOptions{})
		return err
	case err != nil:
		return err
	}

	existing.Spec.HolderIdentity = &holder
	existing.Spec.LeaseDurationSeconds = &dur
	existing.Spec.RenewTime = &now
	_, err = leases.Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// sanitizeForDNS strips characters disallowed in metadata.name. The
// label rules (RFC 1123) only allow [a-z0-9-]. We lowercase and
// replace everything else with '-'.
func sanitizeForDNS(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			r = '-'
		}
		b.WriteRune(r)
	}
	return strings.Trim(b.String(), "-")
}

// agentName composes a deterministic-but-unique CR name from identity
// and a short session prefix.
func agentName(identity, sessionID string) string {
	id := sanitizeForDNS(identity)
	sid := sanitizeForDNS(sessionID)
	if len(sid) > 8 {
		sid = sid[:8]
	}
	if id == "" {
		id = "anon"
	}
	return fmt.Sprintf("%s-%s", id, sid)
}

func (c *realClient) CreateAgent(ctx context.Context, identity, sessionID string) (string, error) {
	name := agentName(identity, sessionID)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "xpos.xpos-it.com/v1alpha1",
		"kind":       "Agent",
		"metadata": map[string]any{
			"name":      name,
			"namespace": c.namespace,
		},
		"spec": map[string]any{
			"identity":  identity,
			"sessionID": sessionID,
			"relayPod": map[string]any{
				"namespace": c.namespace,
				"name":      c.podName,
			},
		},
	}}
	_, err := c.dynamic.Resource(agentGVR).Namespace(c.namespace).
		Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// Same agent reconnected before GC; reuse the name.
		return name, nil
	}
	return name, err
}

func (c *realClient) DeleteAgent(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	err := c.dynamic.Resource(agentGVR).Namespace(c.namespace).
		Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *realClient) CreateTunnel(ctx context.Context, name, hostname, protocol, agentName string) error {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "xpos.xpos-it.com/v1alpha1",
		"kind":       "Tunnel",
		"metadata": map[string]any{
			"name":      name,
			"namespace": c.namespace,
		},
		"spec": map[string]any{
			"protocol": protocol,
			"hostname": hostname,
			"agentRef": map[string]any{
				"name":      agentName,
				"namespace": c.namespace,
			},
		},
	}}
	_, err := c.dynamic.Resource(tunnelGVR).Namespace(c.namespace).
		Create(ctx, obj, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// WaitTunnelTCPPort polls the Tunnel CR until status.tcpPort is set
// by the operator. Polling (rather than a Watch) keeps the relay's
// k8s footprint minimal — each tunnel handshake is a short-lived
// event so a Watch's setup cost would dominate. The poll cadence is
// tight (250ms) because the operator typically reconciles within a
// few hundred milliseconds of CR creation, and the handshake budget
// is bounded by ctx.
func (c *realClient) WaitTunnelTCPPort(ctx context.Context, name string) (int32, error) {
	if name == "" {
		return 0, fmt.Errorf("WaitTunnelTCPPort: empty name")
	}
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		got, err := c.dynamic.Resource(tunnelGVR).Namespace(c.namespace).
			Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			// status.tcpPort is `*int32` so the operator's
			// unstructured representation is an int64 (json
			// number). nestedInt64 returns (0,false,nil) when
			// the path is absent, which is exactly the "not
			// yet assigned" signal we wait on.
			port, found, ferr := unstructured.NestedInt64(got.Object, "status", "tcpPort")
			if ferr != nil {
				return 0, fmt.Errorf("read status.tcpPort: %w", ferr)
			}
			if found && port > 0 {
				return int32(port), nil
			}
		} else if !apierrors.IsNotFound(err) {
			// Transient errors (timeouts, throttling) are
			// retried on the next tick; only fail fast on
			// definitive deletion or context cancellation.
			c.logger.Warn("poll tunnel for tcpPort", "err", err.Error())
		}

		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-t.C:
		}
	}
}

func (c *realClient) DeleteTunnel(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	err := c.dynamic.Resource(tunnelGVR).Namespace(c.namespace).
		Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
