// Command sample-app times every pod from creation to Ready, keeps the times in
// PostgreSQL, and serves a page with their percentiles by namespace, over time.
//
//	sample-app [--addr :8080]   in the cluster; the database comes from the PG* environment
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/apple/container-examples/container-cluster-pod-startup/internal/controller"
	"github.com/apple/container-examples/container-cluster-pod-startup/internal/tracker"
	"github.com/apple/container-examples/container-cluster-pod-startup/internal/web"
)

// percentiles are the numbers the page shows beside the chart.
var percentiles = []float64{50, 95}

func main() {
	addr := flag.String("addr", ":8080", "address of the page and its API")
	kubeContext := flag.String("context", "", "kubeconfig context; empty means in-cluster or the current one")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *addr, *kubeContext); err != nil {
		fmt.Fprintln(os.Stderr, "sample-app:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, addr, kubeContext string) error {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctrl.SetLogger(logr.FromSlogHandler(log.Handler()))

	cfg, err := config.GetConfigWithContext(kubeContext)
	if err != nil {
		return err
	}
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 clientgoscheme.Scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
	})
	if err != nil {
		return err
	}
	queue := tracker.NewQueue(&tracker.Postgres{})
	if err := (&controller.PodReconciler{Client: mgr.GetClient(), Queue: queue}).SetupWithManager(mgr); err != nil {
		return err
	}
	go queue.Run(ctx)
	srv := web.New(queue, percentiles)
	srv.Namespace, srv.Started = ownNamespace(), time.Now()
	errc := make(chan error, 2)
	go func() { errc <- web.Serve(ctx, addr, srv.Handler()) }()
	go func() { errc <- mgr.Start(ctx) }()
	log.Info("watching pods", "addr", addr)
	if err := <-errc; err != nil && ctx.Err() == nil {
		return err
	}
	return nil
}

// ownNamespace is the namespace the app runs in: POD_NAMESPACE from the downward API, else
// the service account's namespace file, else empty out of a cluster.
func ownNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	b, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	return strings.TrimSpace(string(b))
}
