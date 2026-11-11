package tracker

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestPostgres runs against a real database when PG_URL is set, for example
// PG_URL=postgres://postgres:secret@localhost:5432/postgres?sslmode=disable.
func TestPostgres(t *testing.T) {
	url := os.Getenv("PG_URL")
	if url == "" {
		t.Skip("PG_URL not set")
	}
	ctx := context.Background()
	p := &Postgres{URL: url}
	run := fmt.Sprint(time.Now().UnixNano())
	var rows []Row
	for i := 0; i < 3; i++ {
		rows = append(rows, Row{UID: fmt.Sprintf("test-%s-%d", run, i), Namespace: "default", Name: "hello", Node: "n", Owner: "Deployment/hello",
			Created: time.Now().Add(-time.Hour), ReadyAt: time.Now().Add(time.Duration(i) * time.Hour), Scheduled: 0.2, Ready: float64(i) + 0.5})
	}
	if err := p.Save(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if err := p.Save(ctx, rows[:1]); err != nil { // a repeated row is a no-op
		t.Fatal(err)
	}
	got, err := p.Recent(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].UID != rows[2].UID || got[0].Ready != 0.5 {
		t.Fatalf("Recent = %+v", got)
	}
}
