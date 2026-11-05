package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/apple/container-examples/container-cluster-pod-startup/internal/memstore"
	"github.com/apple/container-examples/container-cluster-pod-startup/internal/tracker"
)

// envtestVersion matches ENVTEST_K8S_VERSION in the Makefile.
const envtestVersion = "1.37.0"

var (
	cfg  *rest.Config
	k8s  client.Client
	skip string
)

// TestMain starts one envtest API server. It uses KUBEBUILDER_ASSETS, or
// bin/k8s/<version>-<os>-<arch> as written by make envtest, and never the cluster in the
// kubeconfig, whatever USE_EXISTING_CLUSTER says.
func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		dir := filepath.Join("..", "..", "bin", "k8s", fmt.Sprintf("%s-%s-%s", envtestVersion, runtime.GOOS, runtime.GOARCH))
		if _, err := os.Stat(filepath.Join(dir, "kube-apiserver")); err != nil {
			skip = "envtest binaries not found: run make envtest or make test"
			os.Exit(m.Run())
		}
		abs, _ := filepath.Abs(dir)
		os.Setenv("KUBEBUILDER_ASSETS", abs)
	}
	env := &envtest.Environment{UseExistingCluster: ptr.To(false)}
	var err error
	if cfg, err = env.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "envtest:", err)
		os.Exit(1)
	}
	k8s, err = client.New(cfg, client.Options{Scheme: clientgoscheme.Scheme})
	if err != nil {
		fmt.Fprintln(os.Stderr, "client:", err)
		os.Exit(1)
	}
	code := m.Run()
	_ = env.Stop()
	os.Exit(code)
}

// start runs a manager with the reconciler until the test ends.
func start(t *testing.T) *tracker.Queue {
	t.Helper()
	if skip != "" {
		t.Skip(skip)
	}
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: clientgoscheme.Scheme, Metrics: metricsserver.Options{BindAddress: "0"}, HealthProbeBindAddress: "0",
		Controller: ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
	})
	if err != nil {
		t.Fatal(err)
	}
	q := tracker.NewQueue(&memstore.Store{})
	if err := (&PodReconciler{Client: mgr.GetClient(), Queue: q}).SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = mgr.Start(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		t.Fatal("cache did not sync")
	}
	return q
}

func newPod(t *testing.T, name string) *corev1.Pod {
	t.Helper()
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name, Labels: map[string]string{"app": "hello"}},
		Spec:       corev1.PodSpec{NodeName: "local-cluster1-worker-1", Containers: []corev1.Container{{Name: "hello", Image: "registry.k8s.io/pause:3.10.1"}}},
	}
	if err := k8s.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = k8s.Delete(context.Background(), p, client.GracePeriodSeconds(0)) })
	return p
}

// setReady writes the status a kubelet would: scheduled at creation, Ready at readyAt.
func setReady(t *testing.T, p *corev1.Pod, readyAt time.Time) {
	t.Helper()
	p.Status.Phase = corev1.PodRunning
	p.Status.Conditions = []corev1.PodCondition{
		{Type: corev1.PodScheduled, Status: corev1.ConditionTrue, LastTransitionTime: p.CreationTimestamp},
		{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: metav1.NewTime(readyAt)},
	}
	if err := k8s.Status().Update(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func rowFor(t *testing.T, q *tracker.Queue, uid string) tracker.Row {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		q.Tick(context.Background())
		rows, _, _ := q.State()
		for _, r := range rows {
			if r.UID == uid {
				return r
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no row for %s", uid)
	return tracker.Row{}
}

func TestPodCreatedAfterStartIsTimedByObservation(t *testing.T) {
	q := start(t)
	p := newPod(t, "hello-live")
	time.Sleep(500 * time.Millisecond) // the reconciler sees it pending first
	q.Tick(context.Background())
	if rows, _, pending := q.State(); len(rows) != 0 || pending != 0 {
		t.Fatalf("a pending pod was recorded: %d rows, %d pending", len(rows), pending)
	}
	setReady(t, p, time.Now())
	r := rowFor(t, q, string(p.UID))
	if r.Name != "hello-live" || r.Namespace != "default" || r.Node != "local-cluster1-worker-1" {
		t.Fatalf("row = %+v", r)
	}
	// Timed on the local clock: a fraction of a second, which the API's whole-second
	// condition times could not give (they would say 0 or 1).
	if r.Ready <= 0.1 || r.Ready >= 1 {
		t.Fatalf("ready seconds = %v, want about the half second the pod was pending", r.Ready)
	}
	// Later updates of a Ready pod add nothing.
	p.Labels["touched"] = "yes"
	if err := k8s.Update(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, _, pending := q.State(); pending != 0 {
		t.Fatalf("an update re-queued the pod: %d pending", pending)
	}
}

func TestPodRunningAtStartIsTimedByItsConditions(t *testing.T) {
	if skip != "" {
		t.Skip(skip)
	}
	p := newPod(t, "hello-backfill")
	setReady(t, p, p.CreationTimestamp.Add(3*time.Second))
	q := start(t) // the manager starts after the pod is already Ready
	r := rowFor(t, q, string(p.UID))
	if r.Ready != 3 || r.Scheduled != 0 {
		t.Fatalf("backfilled row = %+v; want 3 s from the condition times", r)
	}
}
