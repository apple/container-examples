package tracker

import (
	"math"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var t0 = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

func pod(ready bool, readyAt, scheduledAt time.Time) *corev1.Pod {
	p := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		UID: "u1", Namespace: "default", Name: "hello-7d9c5b8f6-x2k9q", CreationTimestamp: metav1.NewTime(t0),
		Labels:          map[string]string{"pod-template-hash": "7d9c5b8f6"},
		OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "hello-7d9c5b8f6", Controller: ptr.To(true)}},
	}, Spec: corev1.PodSpec{NodeName: "local-cluster1-worker-1"}}
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	p.Status.Conditions = []corev1.PodCondition{
		{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(scheduledAt)},
		{Type: corev1.PodReady, Status: status, LastTransitionTime: metav1.NewTime(readyAt)},
	}
	return p
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// started is when the controller started: one minute before t0, the pods' creation.
var started = t0.Add(-time.Minute)

func TestPodCreatedAfterStartIsTimedByObservation(t *testing.T) {
	seen := Seen{First: t0.Add(200 * time.Millisecond), Scheduled: t0.Add(450 * time.Millisecond)}
	now := t0.Add(1650 * time.Millisecond) // when the controller saw it Ready
	// The Ready condition says 2 s; the observations say 1.45 s, and they win.
	r, ok := Measure(pod(true, t0.Add(2*time.Second), t0), seen, started, now)
	if !ok || !near(r.Ready, 1.45) || !near(r.Scheduled, 0.25) || !r.ReadyAt.Equal(now) {
		t.Fatalf("row = %+v, %v", r, ok)
	}
	if r.UID != "u1" || r.Node != "local-cluster1-worker-1" || r.Owner != "Deployment/hello" || !r.Created.Equal(t0) {
		t.Fatalf("identity = %+v", r)
	}
}

func TestPodRunningAtStartIsTimedByItsConditions(t *testing.T) {
	// Created before the controller started: creation timestamp to Ready condition, even
	// when the controller saw it before it became Ready.
	later := t0.Add(time.Minute)
	for _, seen := range []Seen{{}, {First: later, Scheduled: later}} {
		r, ok := Measure(pod(true, t0.Add(3*time.Second), t0.Add(time.Second)), seen, later, t0.Add(time.Hour))
		if !ok || !near(r.Ready, 3) || !near(r.Scheduled, 1) || !r.ReadyAt.Equal(t0.Add(3*time.Second)) {
			t.Fatalf("seen %+v: row = %+v, %v", seen, r, ok)
		}
	}
}

func TestMeasureSkipsPodsThatAreNotReady(t *testing.T) {
	if _, ok := Measure(pod(false, t0, t0), Seen{}, started, t0); ok {
		t.Fatal("not-ready pod measured")
	}
	if _, ok := Measure(&corev1.Pod{}, Seen{}, started, t0); ok {
		t.Fatal("pod without conditions measured")
	}
	// A clock skew never gives a negative time.
	if r, _ := Measure(pod(true, t0.Add(-time.Second), t0.Add(-time.Second)), Seen{}, started, t0); r.Ready != 0 || r.Scheduled != 0 {
		t.Fatalf("negative durations = %+v", r)
	}
}

func TestOwner(t *testing.T) {
	p := pod(true, t0, t0)
	if got := Owner(p); got != "Deployment/hello" {
		t.Fatalf("Owner = %q", got)
	}
	p.OwnerReferences[0] = metav1.OwnerReference{Kind: "DaemonSet", Name: "cilium", Controller: ptr.To(true)}
	if got := Owner(p); got != "DaemonSet/cilium" {
		t.Fatalf("Owner = %q", got)
	}
	p.OwnerReferences = nil
	if got := Owner(p); got != "" {
		t.Fatalf("Owner = %q", got)
	}
}

func TestPercentile(t *testing.T) {
	v := []float64{4, 1, 3, 2, 5}
	for _, c := range []struct{ p, want float64 }{{0, 1}, {50, 3}, {100, 5}, {25, 2}, {95, 4.8}, {99, 4.96}} {
		if got := Percentile(v, c.p); !near(got, c.want) {
			t.Errorf("p%v = %v, want %v", c.p, got, c.want)
		}
	}
	if Percentile(nil, 50) != 0 || Percentile([]float64{2.5}, 95) != 2.5 {
		t.Error("empty or single-value percentile")
	}
	if v[0] != 4 {
		t.Error("Percentile sorted its input")
	}
}
