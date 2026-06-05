// Package k8s contains the relay's optional Kubernetes integration:
//   - A coordination.k8s.io Lease that the relay pod renews to advertise
//     its liveness to the operator.
//   - Lifecycle management of xpos.xpos-it.com/v1alpha1 Agent and
//     Tunnel custom resources (one CR per active session / tunnel).
//
// Everything in this package gracefully degrades to a no-op when the
// relay is running outside a cluster, so the same binary works on a
// laptop without code changes. Use NewClient and dispatch on the
// returned Client interface; the no-op implementation is returned when
// rest.InClusterConfig fails.
//
// We intentionally use the dynamic client (unstructured) for the xpos
// CRDs to avoid pulling the operator Go module into the relay's
// dependency graph. The Lease uses the typed coordv1 client.
package k8s

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Client is the surface the relay calls into. Implementations:
//   - realClient  : talks to the API server.
//   - noopClient  : returned when out-of-cluster.
type Client interface {
	// Start begins the Lease renewal loop. It returns immediately
	// after first renewal; the loop runs until ctx is cancelled.
	Start(ctx context.Context) error

	// CreateAgent creates an Agent CR for this session. The returned
	// name is what was actually stored (after sanitization).
	CreateAgent(ctx context.Context, identity, sessionID string) (name string, err error)

	// DeleteAgent removes an Agent CR previously returned by
	// CreateAgent. Missing-not-found is not an error.
	DeleteAgent(ctx context.Context, name string) error

	// CreateTunnel creates a Tunnel CR with the given protocol and
	// hostname, referencing the named Agent.
	CreateTunnel(ctx context.Context, name, hostname, protocol, agentName string) error

	// DeleteTunnel removes a Tunnel CR. Missing-not-found is not an
	// error.
	DeleteTunnel(ctx context.Context, name string) error

	// WaitTunnelTCPPort polls the named Tunnel CR until the
	// operator has set status.tcpPort, then returns that port.
	// Returns an error if ctx is cancelled before the port is
	// assigned, or if the Tunnel disappears. Callers pass a
	// context with a deadline appropriate to their handshake
	// budget.
	WaitTunnelTCPPort(ctx context.Context, name string) (int32, error)

	// PodName returns this pod's name (empty when out-of-cluster).
	PodName() string
	// Namespace returns this pod's namespace (empty when out-of-cluster).
	Namespace() string
}

// agentGVR / tunnelGVR are the resource handles used with the dynamic
// client. Kept in one place so a future API bump is one-line.
var (
	agentGVR = schema.GroupVersionResource{
		Group: "xpos.xpos-it.com", Version: "v1alpha1", Resource: "agents",
	}
	tunnelGVR = schema.GroupVersionResource{
		Group: "xpos.xpos-it.com", Version: "v1alpha1", Resource: "tunnels",
	}
)

// NewClient returns a real Client if rest.InClusterConfig succeeds and
// the downward-API env vars are present; otherwise a no-op Client.
// The returned Client is always non-nil.
func NewClient(logger *slog.Logger) (Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		logger.Info("k8s integration disabled (not in cluster)", "err", err.Error())
		return noopClient{}, nil
	}

	podName := os.Getenv("XPOS_POD_NAME")
	podNS := os.Getenv("XPOS_POD_NAMESPACE")
	if podName == "" || podNS == "" {
		return nil, fmt.Errorf("XPOS_POD_NAME and XPOS_POD_NAMESPACE must be set in-cluster")
	}

	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build typed clientset: %w", err)
	}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}

	return &realClient{
		logger:    logger,
		typed:     kc,
		dynamic:   dc,
		podName:   podName,
		namespace: podNS,
	}, nil
}
