// Package tracker measures how long pods take to start and keeps the measurements on
// their way to the database.
package tracker

import (
	"math"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Row is one pod start.
// Scheduled and Ready are seconds from the pod's creation.
type Row struct {
	UID       string    `json:"uid"`
	Namespace string    `json:"namespace"`
	Name      string    `json:"name"`
	Node      string    `json:"node"`
	Owner     string    `json:"owner"`
	Created   time.Time `json:"created"`
	ReadyAt   time.Time `json:"readyAt"`
	Scheduled float64   `json:"scheduled"`
	Ready     float64   `json:"ready"`
}

// Seen is what the controller observed of a pod: when it first saw it, and when it first
// saw it scheduled.
type Seen struct {
	First, Scheduled time.Time
}

// Measure times a Ready pod: created after start, first seen to Ready seen (ms); else creation to its Ready condition (s).
func Measure(p *corev1.Pod, seen Seen, started, now time.Time) (Row, bool) {
	ready := condition(p, corev1.PodReady)
	if ready == nil || ready.Status != corev1.ConditionTrue {
		return Row{}, false
	}
	created := p.CreationTimestamp.Time
	r := Row{
		UID: string(p.UID), Namespace: p.Namespace, Name: p.Name, Node: p.Spec.NodeName,
		Owner: Owner(p), Created: created,
	}
	// Creation timestamps are whole seconds, so compare at that precision.
	if !seen.First.IsZero() && !created.Before(started.Truncate(time.Second)) {
		r.ReadyAt = now
		r.Ready = now.Sub(seen.First).Seconds()
		if !seen.Scheduled.IsZero() {
			r.Scheduled = seen.Scheduled.Sub(seen.First).Seconds()
		}
	} else {
		r.ReadyAt = ready.LastTransitionTime.Time
		r.Ready = r.ReadyAt.Sub(created).Seconds()
		if s := condition(p, corev1.PodScheduled); s != nil && s.Status == corev1.ConditionTrue {
			r.Scheduled = s.LastTransitionTime.Sub(created).Seconds()
		}
	}
	r.Ready, r.Scheduled = math.Max(0, r.Ready), math.Max(0, r.Scheduled)
	return r, true
}

func condition(p *corev1.Pod, t corev1.PodConditionType) *corev1.PodCondition {
	for i := range p.Status.Conditions {
		if p.Status.Conditions[i].Type == t {
			return &p.Status.Conditions[i]
		}
	}
	return nil
}

// Owner names the workload that runs the pod: a Deployment for a ReplicaSet's pod,
// otherwise the controller's kind and name.
func Owner(p *corev1.Pod) string {
	ref := metav1.GetControllerOf(p)
	if ref == nil {
		return ""
	}
	if h := p.Labels["pod-template-hash"]; ref.Kind == "ReplicaSet" && strings.HasSuffix(ref.Name, "-"+h) {
		return "Deployment/" + strings.TrimSuffix(ref.Name, "-"+h)
	}
	return ref.Kind + "/" + ref.Name
}

// Percentile is the p-th percentile of values, interpolated between the closest ranks.
func Percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	rank := p / 100 * float64(len(v)-1)
	lo := int(math.Floor(rank))
	if lo >= len(v)-1 {
		return v[len(v)-1]
	}
	return v[lo] + (v[lo+1]-v[lo])*(rank-float64(lo))
}
