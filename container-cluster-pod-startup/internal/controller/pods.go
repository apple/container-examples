// Package controller holds the one reconciler: it times every pod from creation to Ready.
package controller

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/apple/container-examples/container-cluster-pod-startup/internal/tracker"
)

// PodReconciler records one row per pod when it becomes Ready. The first list after start
// reconciles every existing pod, so pods already running are backfilled.
type PodReconciler struct {
	client.Client
	Queue *tracker.Queue
	Now   func() time.Time

	mu      sync.Mutex
	started time.Time
	pods    map[types.NamespacedName]*podState
}

type podState struct {
	uid  types.UID
	seen tracker.Seen
	done bool
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// Reconcile implements reconcile.Reconciler.
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			r.mu.Lock()
			delete(r.pods, req.NamespacedName)
			r.mu.Unlock()
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	now := r.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.pods[req.NamespacedName]
	if st == nil || st.uid != pod.UID { // a new pod, or a new pod with the same name
		st = &podState{uid: pod.UID}
		r.pods[req.NamespacedName] = st
	}
	if st.done {
		return ctrl.Result{}, nil
	}
	if row, ok := tracker.Measure(&pod, st.seen, r.started, now); ok {
		r.Queue.Add(row)
		st.done = true
		return ctrl.Result{}, nil
	}
	if st.seen.First.IsZero() {
		st.seen.First = now
	}
	if st.seen.Scheduled.IsZero() && pod.Spec.NodeName != "" {
		st.seen.Scheduled = now
	}
	return ctrl.Result{}, nil
}

// SetupWithManager registers the reconciler for pods in every namespace.
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.pods = map[types.NamespacedName]*podState{}
	if r.Now == nil {
		r.Now = time.Now
	}
	r.started = r.Now()
	return ctrl.NewControllerManagedBy(mgr).For(&corev1.Pod{}).Named("pod").Complete(r)
}
