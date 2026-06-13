package controller

import (
	"context"
	"time"

	coordv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	xposv1alpha1 "github.com/ametow/xpos/operator/api/v1alpha1"
)

// HeartbeatGracePeriod is how long after a relay's Lease has gone
// stale we wait before declaring its Agents orphaned and deleting them.
// This buffers against transient apiserver/relay flakes.
const HeartbeatGracePeriod = 30 * time.Second

// AgentReconciler reconciles Agent objects, mainly garbage collecting
// agents whose owning relay pod has stopped renewing its Lease.
type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=xpos.xpos-io.com,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=xpos.xpos-io.com,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=xpos.xpos-io.com,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var agent xposv1alpha1.Agent
	if err := r.Get(ctx, req.NamespacedName, &agent); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Look up the relay pod's Lease (one Lease per relay pod, named
	// after the pod) to decide whether this Agent is still live.
	var lease coordv1.Lease
	leaseKey := types.NamespacedName{
		Namespace: agent.Spec.RelayPod.Namespace,
		Name:      agent.Spec.RelayPod.Name,
	}
	err := r.Get(ctx, leaseKey, &lease)
	switch {
	case apierrors.IsNotFound(err):
		// No lease at all -> the relay never came up, or has been
		// gone long enough that its Lease was GCed. Treat as orphan.
		logger.Info("relay lease missing, deleting orphan agent",
			"relay", leaseKey.String(),
		)
		return ctrl.Result{}, r.Delete(ctx, &agent)
	case err != nil:
		return ctrl.Result{}, err
	}

	now := time.Now()
	if isLeaseStale(&lease, now) {
		logger.Info("relay lease stale, deleting orphan agent",
			"relay", leaseKey.String(),
		)
		return ctrl.Result{}, r.Delete(ctx, &agent)
	}

	// Mirror lease renewTime into status for human consumption.
	if lease.Spec.RenewTime != nil {
		mt := metav1.NewTime(lease.Spec.RenewTime.Time)
		if agent.Status.LastHeartbeat == nil ||
			!agent.Status.LastHeartbeat.Equal(&mt) {
			agent.Status.LastHeartbeat = &mt
			if err := r.Status().Update(ctx, &agent); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Requeue so we re-check liveness even if no Lease event arrives.
	return ctrl.Result{RequeueAfter: HeartbeatGracePeriod}, nil
}

func isLeaseStale(l *coordv1.Lease, now time.Time) bool {
	if l.Spec.RenewTime == nil {
		return true
	}
	dur := HeartbeatGracePeriod
	if l.Spec.LeaseDurationSeconds != nil {
		dur += time.Duration(*l.Spec.LeaseDurationSeconds) * time.Second
	}
	return now.Sub(l.Spec.RenewTime.Time) > dur
}

// agentsForRelayPodKey returns reconcile requests for every Agent
// whose spec.relayPod matches (namespace, name). Shared by the Lease
// and Pod watch handlers since both reference a relay pod by NS/Name.
func (r *AgentReconciler) agentsForRelayPodKey(ctx context.Context, ns, name string) []reconcile.Request {
	var agents xposv1alpha1.AgentList
	if err := r.List(ctx, &agents); err != nil {
		log.FromContext(ctx).Error(err, "list agents",
			"relay", ns+"/"+name)
		return nil
	}
	out := make([]reconcile.Request, 0, len(agents.Items))
	for i := range agents.Items {
		a := &agents.Items[i]
		if a.Spec.RelayPod.Name == name &&
			a.Spec.RelayPod.Namespace == ns {
			out = append(out, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: a.Namespace,
					Name:      a.Name,
				},
			})
		}
	}
	return out
}

func (r *AgentReconciler) agentsForLease(ctx context.Context, obj client.Object) []reconcile.Request {
	lease, ok := obj.(*coordv1.Lease)
	if !ok {
		return nil
	}
	return r.agentsForRelayPodKey(ctx, lease.Namespace, lease.Name)
}

// agentsForPod fires when a relay Pod is deleted or transitions out
// of Ready, giving us a tighter signal than waiting for the Lease to
// expire. We deliberately don't filter by label here: the Agent's
// spec.relayPod is the source of truth, and matching by it costs at
// most one List per pod event.
func (r *AgentReconciler) agentsForPod(ctx context.Context, obj client.Object) []reconcile.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}
	return r.agentsForRelayPodKey(ctx, pod.Namespace, pod.Name)
}

func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&xposv1alpha1.Agent{}).
		Watches(
			&coordv1.Lease{},
			handler.EnqueueRequestsFromMapFunc(r.agentsForLease),
			builder.WithPredicates(),
		).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.agentsForPod),
			builder.WithPredicates(),
		).
		Complete(r)
}
