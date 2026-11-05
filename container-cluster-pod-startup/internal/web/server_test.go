package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apple/container-examples/container-cluster-pod-startup/internal/memstore"
	"github.com/apple/container-examples/container-cluster-pod-startup/internal/tracker"
)

func server(t *testing.T, down bool, percentiles ...float64) *Server {
	t.Helper()
	store := &memstore.Store{}
	q := tracker.NewQueue(store)
	for i, s := range []float64{1.0, 1.2, 1.4, 2.0, 4.0} {
		q.Add(tracker.Row{UID: string(rune('a' + i)), ReadyAt: time.Unix(int64(i), 0), Ready: s})
	}
	q.Tick(context.Background())
	store.SetDown(down)
	if down {
		q.Tick(context.Background())
	}
	if len(percentiles) == 0 {
		percentiles = []float64{50, 95}
	}
	return New(q, percentiles)
}

func TestState(t *testing.T) {
	srv := httptest.NewServer(server(t, false).Handler())
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var st State
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st.Database != "connected" || len(st.Rows) != 5 || st.Rows[4].Ready != 4.0 {
		t.Fatalf("state = %+v", st)
	}
	if len(st.Percentiles) != 2 || st.Percentiles[0] != (Value{50, 1.4}) || st.Percentiles[1] != (Value{95, 3.6}) {
		t.Fatalf("percentiles = %+v", st.Percentiles)
	}
}

func TestStateWhileTheDatabaseIsUnreachable(t *testing.T) {
	st := server(t, true).State()
	if st.Database != "unreachable" || len(st.Rows) != 5 {
		t.Fatalf("state = %+v; want the last rows kept and the status unreachable", st)
	}
}

func TestThePageRendersWhateverPercentilesTheServerSends(t *testing.T) {
	s := server(t, false, 50, 95, 99)
	if got := s.State().Percentiles; len(got) != 3 || got[2].P != 99 {
		t.Fatalf("percentiles = %+v", got)
	}
}

func TestNamespacesOwnFirstThenByPods(t *testing.T) {
	store := &memstore.Store{}
	q := tracker.NewQueue(store)
	for i, r := range []struct {
		ns    string
		ready float64
	}{{"kube-system", 2}, {"kube-system", 4}, {"kube-system", 30}, {"default", 1}, {"default", 3}, {"sample-controller", 12}, {"monitoring", 5}} {
		q.Add(tracker.Row{UID: string(rune('a' + i)), Namespace: r.ns, ReadyAt: time.Unix(int64(i), 0), Ready: r.ready})
	}
	q.Tick(context.Background())
	s := New(q, []float64{50, 95})
	s.Namespace = "sample-controller"
	var got []string
	for _, n := range s.State().Namespaces {
		got = append(got, n.Name)
	}
	if strings.Join(got, ",") != "sample-controller,kube-system,default,monitoring" {
		t.Fatalf("namespaces = %v", got)
	}
	ks := s.State().Namespaces[1]
	if ks.Pods != 3 || len(ks.Percentiles) != 2 || ks.Percentiles[0] != (Value{50, 4}) {
		t.Fatalf("kube-system = %+v", ks)
	}
}

func TestEvents(t *testing.T) {
	srv := httptest.NewServer(server(t, false).Handler())
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/events", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	if !sc.Scan() || sc.Text() != "event: state" || !sc.Scan() || !strings.HasPrefix(sc.Text(), "data: {") {
		t.Fatalf("first frame = %q", sc.Text())
	}
	var st State
	if err := json.Unmarshal([]byte(strings.TrimPrefix(sc.Text(), "data: ")), &st); err != nil || len(st.Rows) != 5 {
		t.Fatalf("frame = %v, %v", st, err)
	}
}

func TestHealthzAndPage(t *testing.T) {
	srv := httptest.NewServer(server(t, false).Handler())
	defer srv.Close()
	for path, want := range map[string]string{"/healthz": "ok", "/": "Pod startup time", "/app.js": "EventSource", "/app.css": "--orange"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		body := string(b)
		if res.StatusCode != 200 || !strings.Contains(body, want) {
			t.Errorf("GET %s = %d, missing %q", path, res.StatusCode, want)
		}
		// The page never names the app and never reaches off the box.
		for _, bad := range []string{"sample-app", "sample app", "http://", "https://", "@font-face", "@import"} {
			if strings.Contains(strings.ToLower(body), bad) {
				t.Errorf("GET %s contains %q", path, bad)
			}
		}
	}
}
