package controller

// RelayPodReconciler keeps a per-pod Service in sync with each running
// relay Pod. Why this exists:
//
// Gateway API HTTPRoute backendRefs must point at a Service (you can't
// reference a Pod directly). To route a specific tunnel to the
// specific relay pod that holds its agent connection, we need a
// Service that selects only that pod. The relay runs as a StatefulSet,
// so the built-in `statefulset.kubernetes.io/pod-name` label provides
// the per-pod selector we need.
//
// The reconciler:
//   - watches Pods labeled `app.kubernetes.io/name=xpos-relay` (the
//     selector is configurable via env);
//   - for each such Pod, ensures a Service of the same name exists
//     with that pod-name selector and the configured HTTP port;
//   - sets the Pod as the owner of the Service so the Service is GCed
//     when the Pod is deleted permanently.

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// RelayPodReconcilerConfig is the runtime configuration for the
// per-pod Service reconciler.
type RelayPodReconcilerConfig struct {
	// PodLabelKey/PodLabelValue identify relay pods. Defaults
	// `app.kubernetes.io/name`=`xpos-relay`.
	PodLabelKey   string
	PodLabelValue string

	// HTTPPort is the targetPort exposed by the per-pod Service.
	HTTPPort int32
}

// RelayPodReconciler reconciles per-pod Services for relay Pods.
type RelayPodReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config RelayPodReconcilerConfig
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *RelayPodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			// Service is owned by the Pod, so it's GCed
			// automatically. Nothing to do.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Skip terminating pods — let the GC remove the Service.
	if pod.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Spec.Selector = map[string]string{
			"statefulset.kubernetes.io/pod-name": pod.Name,
		}
		svc.Spec.Ports = []corev1.ServicePort{{
			Name:       "http",
			Port:       r.Config.HTTPPort,
			TargetPort: intstr.FromInt32(r.Config.HTTPPort),
			Protocol:   corev1.ProtocolTCP,
		}}
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		return controllerutil.SetControllerReference(&pod, svc, r.Scheme)
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if op != controllerutil.OperationResultNone {
		logger.Info("reconciled per-pod service",
			"pod", pod.Name, "op", op)
	}
	return ctrl.Result{}, nil
}

// SetupWithManager wires the reconciler with a label predicate so we
// only see Pods that look like xpos relays.
func (r *RelayPodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	key := r.Config.PodLabelKey
	value := r.Config.PodLabelValue
	hasLabel := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetLabels()[key] == value
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}, builder.WithPredicates(hasLabel)).
		Owns(&corev1.Service{}).
		Complete(r)
}
