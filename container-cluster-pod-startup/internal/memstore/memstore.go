// Package memstore is a tracker.Store in memory, for the tests, which have no database.
// SetDown makes it unreachable.
package memstore

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/apple/container-examples/container-cluster-pod-startup/internal/tracker"
)

// Store keeps rows in memory.
type Store struct {
	mu   sync.Mutex
	rows map[string]tracker.Row
	down bool
}

// SetDown makes the store unreachable, or reachable again.
func (m *Store) SetDown(down bool) {
	m.mu.Lock()
	m.down = down
	m.mu.Unlock()
}

// Save implements tracker.Store.
func (m *Store) Save(_ context.Context, rows []tracker.Row) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.down {
		return errors.New("database unreachable")
	}
	if m.rows == nil {
		m.rows = map[string]tracker.Row{}
	}
	for _, r := range rows {
		if _, ok := m.rows[r.UID]; !ok {
			m.rows[r.UID] = r
		}
	}
	return nil
}

// Recent implements tracker.Store: the n rows that became Ready last, oldest first.
func (m *Store) Recent(_ context.Context, n int) ([]tracker.Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.down {
		return nil, errors.New("database unreachable")
	}
	all := make([]tracker.Row, 0, len(m.rows))
	for _, r := range m.rows {
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ReadyAt.Before(all[j].ReadyAt) })
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
