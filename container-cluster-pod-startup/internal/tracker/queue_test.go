package tracker

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"
)

// fakeStore keeps rows in a map; while down it fails every call.
type fakeStore struct {
	rows map[string]Row
	down bool
}

func (f *fakeStore) Save(_ context.Context, rows []Row) error {
	if f.down {
		return errors.New("down")
	}
	if f.rows == nil {
		f.rows = map[string]Row{}
	}
	for _, r := range rows {
		if _, ok := f.rows[r.UID]; !ok {
			f.rows[r.UID] = r
		}
	}
	return nil
}

func (f *fakeStore) Recent(_ context.Context, n int) ([]Row, error) {
	if f.down {
		return nil, errors.New("down")
	}
	var all []Row
	for _, r := range f.rows {
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ReadyAt.Before(all[j].ReadyAt) })
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}

func row(i int) Row {
	return Row{UID: fmt.Sprint("uid-", i), Name: fmt.Sprint("p", i), ReadyAt: t0.Add(time.Duration(i) * time.Second), Ready: float64(i)}
}

func TestQueueWaitsWhileTheStoreIsDownAndDrainsAfter(t *testing.T) {
	ctx := context.Background()
	store := &fakeStore{}
	q := NewQueue(store)
	q.Add(row(1))
	q.Tick(ctx)
	if rows, up, pending := q.State(); !up || pending != 0 || len(rows) != 1 {
		t.Fatalf("up: rows %d up %v pending %d", len(rows), up, pending)
	}

	store.down = true
	for i := 2; i <= 4; i++ {
		q.Add(row(i))
	}
	q.Add(row(4)) // the same pod twice is one row
	q.Tick(ctx)
	rows, up, pending := q.State()
	if up || pending != 3 || len(rows) != 1 {
		t.Fatalf("down: rows %d up %v pending %d; want the old row shown, 3 waiting", len(rows), up, pending)
	}

	store.down = false
	q.Tick(ctx)
	rows, up, pending = q.State()
	if !up || pending != 0 || len(rows) != 4 || rows[3].UID != "uid-4" {
		t.Fatalf("drained: rows %d up %v pending %d", len(rows), up, pending)
	}
}

func TestQueueShowsTheLastShownOldestFirst(t *testing.T) {
	ctx := context.Background()
	q := NewQueue(&fakeStore{})
	for i := Shown + 10; i >= 1; i-- {
		q.Add(row(i))
	}
	q.Tick(ctx)
	rows, _, _ := q.State()
	if len(rows) != Shown || rows[0].UID != row(11).UID || rows[Shown-1].UID != row(Shown+10).UID {
		t.Fatalf("rows = %d, first %s, last %s", len(rows), rows[0].UID, rows[len(rows)-1].UID)
	}
}

// slowStore never answers; Tick must give up after its timeout, not hang.
type slowStore struct{}

func (slowStore) Save(ctx context.Context, _ []Row) error { <-ctx.Done(); return ctx.Err() }
func (slowStore) Recent(ctx context.Context, _ int) ([]Row, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestTickTimesOut(t *testing.T) {
	q := NewQueue(slowStore{})
	q.Add(row(1))
	start := time.Now()
	q.Tick(context.Background())
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("Tick took %v", d)
	}
	if _, up, pending := q.State(); up || pending != 1 {
		t.Fatalf("up %v pending %d", up, pending)
	}
}
