// Package web serves the page, its state as JSON and as server-sent events, and /healthz.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/apple/container-examples/container-cluster-pod-startup/internal/tracker"
)

//go:embed static
var static embed.FS

// Server is the app's one HTTP server.
type Server struct {
	Queue *tracker.Queue
	// Namespace is the app's own namespace; the page names it and puts it first.
	Namespace string
	// Started is when the controller started; the chart marks it.
	Started time.Time

	percentiles []float64
}

// New returns a server that shows the given percentiles.
func New(q *tracker.Queue, percentiles []float64) *Server {
	return &Server{Queue: q, percentiles: percentiles}
}

// State is what the page renders.
type State struct {
	Namespace   string        `json:"namespace"`
	Started     time.Time     `json:"started"`
	Rows        []tracker.Row `json:"rows"`
	Percentiles []Value       `json:"percentiles"`
	Namespaces  []Namespace   `json:"namespaces"`
	Database    string        `json:"database"` // "connected" or "unreachable"
	Pending     int           `json:"pending"`
}

// Namespace is one namespace's pods and their percentiles.
type Namespace struct {
	Name        string  `json:"name"`
	Pods        int     `json:"pods"`
	Percentiles []Value `json:"percentiles"`
}

// Value is one percentile: P is 50 for p50, Seconds is rounded to a tenth.
type Value struct {
	P       float64 `json:"p"`
	Seconds float64 `json:"seconds"`
}

// State computes the state: the last rows from the database and their percentiles, for
// all of them and per namespace. The app's own namespace comes first, then the others by
// pods, most first.
func (s *Server) State() State {
	rows, up, pending := s.Queue.State()
	st := State{Namespace: s.Namespace, Started: s.Started, Rows: rows, Database: "unreachable", Pending: pending, Namespaces: []Namespace{}}
	if up {
		st.Database = "connected"
	}
	if st.Rows == nil {
		st.Rows = []tracker.Row{}
	}
	ready := make([]float64, len(rows))
	byNS := map[string][]float64{}
	for i, r := range rows {
		ready[i] = r.Ready
		byNS[r.Namespace] = append(byNS[r.Namespace], r.Ready)
	}
	st.Percentiles = values(ready, s.percentiles)
	for name, v := range byNS {
		st.Namespaces = append(st.Namespaces, Namespace{Name: name, Pods: len(v), Percentiles: values(v, s.percentiles)})
	}
	sort.Slice(st.Namespaces, func(i, j int) bool {
		a, b := st.Namespaces[i], st.Namespaces[j]
		if (a.Name == s.Namespace) != (b.Name == s.Namespace) {
			return a.Name == s.Namespace
		}
		if a.Pods != b.Pods {
			return a.Pods > b.Pods
		}
		return a.Name < b.Name
	})
	return st
}

func values(ready, percentiles []float64) []Value {
	out := []Value{}
	for _, p := range percentiles {
		out = append(out, Value{P: p, Seconds: math.Round(tracker.Percentile(ready, p)*10) / 10})
	}
	return out
}

// Handler returns the routes.
func (s *Server) Handler() http.Handler {
	files, _ := fs.Sub(static, "static")
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(files))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "ok") })
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.State())
	})
	mux.HandleFunc("GET /api/events", s.events)
	return mux
}

// events sends the state when the page connects and again whenever it changes, checking
// four times a second.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var last []byte
	for {
		b, _ := json.Marshal(s.State())
		if string(b) != string(last) {
			if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", b); err != nil {
				return
			}
			flusher.Flush()
			last = b
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

// Serve runs h on addr until ctx ends.
func Serve(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}
