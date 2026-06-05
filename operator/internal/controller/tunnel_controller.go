// Package controller hosts the reconcilers for xpos CRDs.
//
// Tunnel reconciliation policy:
//
//   - Resolve spec.agentRef -> Agent. If absent or its relay pod is
//     unknown, mark the Tunnel Pending and requeue.
//   - Mirror the resolved relay pod into status.assignedPod and derive
//     status.publicAddr from spec.hostname (HTTP) or
//     `<hostname>:<port>` (TCP).
//   - For HTTP tunnels: reconcile a Gateway API v1 HTTPRoute named
//     after the Tunnel, owned by the Tunnel, whose parentRefs point
//     at the configured xpos Gateway and whose backendRefs point at
//     a per-pod Service named after the assigned pod.
//   - For TCP tunnels: reconciling a TCPRoute is currently a TODO;
//     status will be set to Pending with reason TCPNotImplemented.
//   - On success: status.phase = Active.
package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	xposv1alpha1 "github.com/ametow/xpos/operator/api/v1alpha1"
)

// TunnelReconcilerConfig holds non-cluster runtime configuration that
// the operator reads from environment at startup (see cmd/main.go).
type TunnelReconcilerConfig struct {
	// GatewayName is the name of the Gateway resource that the
	// generated HTTPRoutes will attach to (parentRefs[0].name).
	GatewayName string
	// GatewayNamespace is the namespace of the target Gateway. Empty
	// means same namespace as the Tunnel.
	GatewayNamespace string
	// RelayHTTPPort is the port on the per-pod relay Service that
	// HTTPRoute backendRefs target.
	RelayHTTPPort int32
	// TCPPortMin/TCPPortMax bound the cluster-wide port range the
	// operator allocates public TCP tunnel ports from. The parent
	// Gateway is expected to expose TCP listeners covering this
	// range; the operator does not write to the Gateway itself.
	TCPPortMin int32
	TCPPortMax int32
}

// TunnelReconciler reconciles a Tunnel object.
type TunnelReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Config    TunnelReconcilerConfig
	Allocator *TCPPortAllocator
}

// +kubebuilder:rbac:groups=xpos.xpos-it.com,resources=tunnels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=xpos.xpos-it.com,resources=tunnels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=xpos.xpos-it.com,resources=tunnels/finalizers,verbs=update
// +kubebuilder:rbac:groups=xpos.xpos-it.com,resources=agents,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes;tcproutes,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main reconciliation loop for Tunnel.
func (r *TunnelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var tn xposv1alpha1.Tunnel
	if err := r.Get(ctx, req.NamespacedName, &tn); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 1. Resolve the referenced Agent.
	agent, err := r.resolveAgent(ctx, &tn)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.markPending(ctx, &tn, "AgentNotFound",
				fmt.Sprintf("Agent %q not found", tn.Spec.AgentRef.Name))
		}
		return ctrl.Result{}, err
	}

	if agent.Spec.RelayPod.Name == "" {
		return r.markPending(ctx, &tn, "RelayPodUnassigned",
			"agent has not been assigned to a relay pod yet")
	}

	// 2. Reflect placement + public address into status.
	tn.Status.AssignedPod = &xposv1alpha1.RelayPodRef{
		Namespace: agent.Spec.RelayPod.Namespace,
		Name:      agent.Spec.RelayPod.Name,
	}
	tn.Status.PublicAddr = tn.Spec.Hostname

	// 3. Reconcile the routing object for this tunnel.
	switch tn.Spec.Protocol {
	case xposv1alpha1.TunnelProtocolHTTP:
		if err := r.reconcileHTTPRoute(ctx, &tn); err != nil {
			return r.markFailed(ctx, &tn, "HTTPRouteReconcile", err.Error())
		}
	case xposv1alpha1.TunnelProtocolTCP:
		if r.Allocator == nil {
			return r.markFailed(ctx, &tn, "TCPAllocatorMissing",
				"TCP routing requires an operator-side port allocator")
		}
		port, err := r.Allocator.Allocate(ctx, &tn)
		if err != nil {
			return r.markPending(ctx, &tn, "TCPPortAllocate", err.Error())
		}
		tn.Status.TCPPort = &port
		tn.Status.PublicAddr = fmt.Sprintf("%s:%d", tn.Spec.Hostname, port)
		if err := r.reconcileTCPRoute(ctx, &tn, port); err != nil {
			return r.markFailed(ctx, &tn, "TCPRouteReconcile", err.Error())
		}
	default:
		return r.markFailed(ctx, &tn, "InvalidProtocol",
			fmt.Sprintf("unsupported protocol %q", tn.Spec.Protocol))
	}

	// 4. Mark active.
	r.setCondition(&tn, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciled",
		Message: "tunnel route is in sync",
	})
	tn.Status.Phase = xposv1alpha1.TunnelPhaseActive
	tn.Status.ObservedGeneration = tn.Generation
	if err := r.Status().Update(ctx, &tn); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("tunnel reconciled",
		"phase", tn.Status.Phase,
		"public", tn.Status.PublicAddr,
		"assigned", tn.Status.AssignedPod.Name,
	)
	return ctrl.Result{}, nil
}

func (r *TunnelReconciler) resolveAgent(ctx context.Context, tn *xposv1alpha1.Tunnel) (*xposv1alpha1.Agent, error) {
	ns := tn.Spec.AgentRef.Namespace
	if ns == "" {
		ns = tn.Namespace
	}
	var agent xposv1alpha1.Agent
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      tn.Spec.AgentRef.Name,
	}, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

// reconcileHTTPRoute creates or updates a single HTTPRoute whose
// backend is the per-pod Service for the assigned relay pod.
//
// Naming: the route is named `<tunnel-name>` in the Tunnel's
// namespace; the backend Service is assumed to be named after the
// assigned pod (provisioned out-of-band by a per-pod Service
// controller — see Phase 4 notes).
func (r *TunnelReconciler) reconcileHTTPRoute(ctx context.Context, tn *xposv1alpha1.Tunnel) error {
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tn.Name,
			Namespace: tn.Namespace,
		},
	}

	gwNamespace := r.Config.GatewayNamespace
	if gwNamespace == "" {
		gwNamespace = tn.Namespace
	}

	hostname := gatewayv1.Hostname(tn.Spec.Hostname)
	port := gatewayv1.PortNumber(r.Config.RelayHTTPPort)
	gwNs := gatewayv1.Namespace(gwNamespace)

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
		route.Spec.ParentRefs = []gatewayv1.ParentReference{{
			Name:      gatewayv1.ObjectName(r.Config.GatewayName),
			Namespace: &gwNs,
		}}
		route.Spec.Hostnames = []gatewayv1.Hostname{hostname}
		route.Spec.Rules = []gatewayv1.HTTPRouteRule{{
			BackendRefs: []gatewayv1.HTTPBackendRef{{
				BackendRef: gatewayv1.BackendRef{
					BackendObjectReference: gatewayv1.BackendObjectReference{
						Name: gatewayv1.ObjectName(tn.Status.AssignedPod.Name),
						Port: &port,
					},
				},
			}},
		}}
		return controllerutil.SetControllerReference(tn, route, r.Scheme)
	})
	return err
}

// reconcileTCPRoute creates or updates a TCPRoute (gateway-api
// v1alpha2) for a TCP tunnel. The route attaches to the configured
// Gateway on a sectionName matching the allocated port, and its
// backend is the per-pod relay Service on the same port.
//
// Naming: like HTTPRoutes, the TCPRoute is named `<tunnel-name>` in
// the Tunnel's namespace and owned by the Tunnel (so deletion of
// the Tunnel garbage-collects the route).
//
// Required Gateway shape: the parent Gateway must already declare a
// TCP listener at `port` (the operator does not write to Gateway).
// The conventional setup is a contiguous block of listeners covering
// the operator's TCPPortMin/Max range; see config/gateway for an
// example.
func (r *TunnelReconciler) reconcileTCPRoute(ctx context.Context, tn *xposv1alpha1.Tunnel, port int32) error {
	route := &gatewayv1a2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tn.Name,
			Namespace: tn.Namespace,
		},
	}

	gwNamespace := r.Config.GatewayNamespace
	if gwNamespace == "" {
		gwNamespace = tn.Namespace
	}
	gwNs := gatewayv1.Namespace(gwNamespace)
	gwPort := gatewayv1.PortNumber(port)
	// sectionName lets a Gateway with multiple listeners be
	// addressed unambiguously. Convention: listener name = "tcp-<port>".
	sectionName := gatewayv1.SectionName(fmt.Sprintf("tcp-%d", port))

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, route, func() error {
		route.Spec.ParentRefs = []gatewayv1a2.ParentReference{{
			Name:        gatewayv1.ObjectName(r.Config.GatewayName),
			Namespace:   &gwNs,
			SectionName: &sectionName,
			Port:        &gwPort,
		}}
		route.Spec.Rules = []gatewayv1a2.TCPRouteRule{{
			BackendRefs: []gatewayv1a2.BackendRef{{
				BackendObjectReference: gatewayv1.BackendObjectReference{
					Name: gatewayv1.ObjectName(tn.Status.AssignedPod.Name),
					Port: &gwPort,
				},
			}},
		}}
		return controllerutil.SetControllerReference(tn, route, r.Scheme)
	})
	return err
}

func (r *TunnelReconciler) markPending(ctx context.Context, tn *xposv1alpha1.Tunnel, reason, msg string) (ctrl.Result, error) {
	tn.Status.Phase = xposv1alpha1.TunnelPhasePending
	r.setCondition(tn, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if err := r.Status().Update(ctx, tn); err != nil {
		return ctrl.Result{}, err
	}
	// Requeue: we depend on external state (Agent) that won't
	// necessarily produce a new event for us.
	return ctrl.Result{RequeueAfter: HeartbeatGracePeriod}, nil
}

func (r *TunnelReconciler) markFailed(ctx context.Context, tn *xposv1alpha1.Tunnel, reason, msg string) (ctrl.Result, error) {
	tn.Status.Phase = xposv1alpha1.TunnelPhaseFailed
	r.setCondition(tn, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: msg,
	})
	if err := r.Status().Update(ctx, tn); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *TunnelReconciler) setCondition(tn *xposv1alpha1.Tunnel, c metav1.Condition) {
	c.LastTransitionTime = metav1.Now()
	c.ObservedGeneration = tn.Generation
	for i, existing := range tn.Status.Conditions {
		if existing.Type == c.Type {
			if existing.Status == c.Status &&
				existing.Reason == c.Reason &&
				existing.Message == c.Message {
				// No-op; preserve original transition time.
				return
			}
			tn.Status.Conditions[i] = c
			return
		}
	}
	tn.Status.Conditions = append(tn.Status.Conditions, c)
}

// tunnelsForAgent maps an Agent change to reconcile requests for any
// Tunnel that references it (by spec.agentRef.name in the Agent's
// namespace, or in the Tunnel's namespace if agentRef.namespace is
// unset). This lets us pick up RelayPod changes on the Agent without
// waiting for the next periodic requeue.
func (r *TunnelReconciler) tunnelsForAgent(ctx context.Context, obj client.Object) []reconcile.Request {
	agent, ok := obj.(*xposv1alpha1.Agent)
	if !ok {
		return nil
	}
	var tunnels xposv1alpha1.TunnelList
	if err := r.List(ctx, &tunnels); err != nil {
		log.FromContext(ctx).Error(err, "list tunnels for agent event",
			"agent", agent.Namespace+"/"+agent.Name)
		return nil
	}
	out := make([]reconcile.Request, 0, len(tunnels.Items))
	for i := range tunnels.Items {
		t := &tunnels.Items[i]
		ns := t.Spec.AgentRef.Namespace
		if ns == "" {
			ns = t.Namespace
		}
		if t.Spec.AgentRef.Name == agent.Name && ns == agent.Namespace {
			out = append(out, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: t.Namespace,
					Name:      t.Name,
				},
			})
		}
	}
	return out
}

// SetupWithManager wires the reconciler into the manager and asks it
// to also requeue Tunnels whose AgentRef matches a changed Agent.
func (r *TunnelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&xposv1alpha1.Tunnel{}).
		Owns(&gatewayv1.HTTPRoute{}).
		Owns(&gatewayv1a2.TCPRoute{}).
		Watches(
			&xposv1alpha1.Agent{},
			handler.EnqueueRequestsFromMapFunc(r.tunnelsForAgent),
		).
		Complete(r)
}
