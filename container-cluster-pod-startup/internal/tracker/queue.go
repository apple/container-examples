package tracker

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Store keeps rows. Save must ignore a row whose UID it already has.
type Store interface {
	Save(ctx context.Context, rows []Row) error
	Recent(ctx context.Context, n int) ([]Row, error)
}

// Shown is how many pod starts the page reads: enough for an hour of a busy cluster.
const Shown = 1000

// Queue holds rows until the store takes them, and the store's latest rows for the page.
// When the store is unreachable, rows wait here; when it returns, they drain.
type Queue struct {
	store   Store
	mu      sync.Mutex
	pending map[string]Row
	rows    []Row
	up      bool
}

// NewQueue returns an empty queue in front of s.
func NewQueue(s Store) *Queue {
	return &Queue{store: s, pending: map[string]Row{}}
}

// Add queues a row; a row already queued is kept once.
func (q *Queue) Add(r Row) {
	q.mu.Lock()
	q.pending[r.UID] = r
	q.mu.Unlock()
}

// State returns the store's latest rows (oldest first), whether the store answered last
// time, and how many rows are waiting.
func (q *Queue) State() (rows []Row, up bool, pending int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]Row(nil), q.rows...), q.up, len(q.pending)
}

// Tick saves what is waiting, then reads back the latest rows. Each call to the store has
// two seconds; a failure marks the store unreachable and leaves the rows queued.
func (q *Queue) Tick(ctx context.Context) {
	q.mu.Lock()
	batch := make([]Row, 0, len(q.pending))
	for _, r := range q.pending {
		batch = append(batch, r)
	}
	q.mu.Unlock()
	sort.Slice(batch, func(i, j int) bool { return batch[i].ReadyAt.Before(batch[j].ReadyAt) })

	err := call(ctx, func(ctx context.Context) error {
		if len(batch) == 0 {
			return nil
		}
		return q.store.Save(ctx, batch)
	})
	var rows []Row
	if err == nil {
		err = call(ctx, func(ctx context.Context) (err error) {
			rows, err = q.store.Recent(ctx, Shown)
			return err
		})
	}

	q.mu.Lock()
	q.up = err == nil
	if err == nil {
		for _, r := range batch {
			delete(q.pending, r.UID)
		}
		q.rows = rows
	}
	q.mu.Unlock()
}

// Run ticks every second until ctx ends.
func (q *Queue) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		q.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func call(ctx context.Context, f func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return f(ctx)
}
