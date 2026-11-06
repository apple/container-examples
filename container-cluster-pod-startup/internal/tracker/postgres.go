package tracker

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const schema = `CREATE TABLE IF NOT EXISTS pod_starts (
	uid               text PRIMARY KEY,
	namespace         text NOT NULL,
	name              text NOT NULL,
	node              text NOT NULL,
	owner             text NOT NULL,
	created_at        timestamptz NOT NULL,
	ready_at          timestamptz NOT NULL,
	scheduled_seconds double precision NOT NULL,
	ready_seconds     double precision NOT NULL
)`

// Postgres is a Store in PostgreSQL. URL is a connection string; empty means the PG*
// environment variables (PGHOST, PGUSER, PGPASSWORD, PGDATABASE).
//
// Every call opens its own connection. A connection kept open could outlive a network
// policy that now blocks the database, and the page would never say so.
type Postgres struct {
	URL string
}

func (p *Postgres) connect(ctx context.Context) (*pgx.Conn, error) {
	c, err := pgx.Connect(ctx, p.URL)
	if err != nil {
		return nil, err
	}
	if _, err := c.Exec(ctx, schema); err != nil {
		c.Close(ctx)
		return nil, err
	}
	return c, nil
}

// Save implements Store; the UID primary key makes a repeated row a no-op.
func (p *Postgres) Save(ctx context.Context, rows []Row) error {
	c, err := p.connect(ctx)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	b := &pgx.Batch{}
	for _, r := range rows {
		b.Queue(`INSERT INTO pod_starts VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT (uid) DO NOTHING`,
			r.UID, r.Namespace, r.Name, r.Node, r.Owner, r.Created, r.ReadyAt, r.Scheduled, r.Ready)
	}
	return c.SendBatch(ctx, b).Close()
}

// Recent implements Store: the n rows that became Ready last, oldest first.
func (p *Postgres) Recent(ctx context.Context, n int) ([]Row, error) {
	c, err := p.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close(ctx)
	rows, err := c.Query(ctx, `SELECT uid, namespace, name, node, owner, created_at, ready_at, scheduled_seconds, ready_seconds
		FROM (SELECT * FROM pod_starts ORDER BY ready_at DESC LIMIT $1) recent ORDER BY ready_at`, n)
	if err != nil {
		return nil, err
	}
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.UID, &r.Namespace, &r.Name, &r.Node, &r.Owner, &r.Created, &r.ReadyAt, &r.Scheduled, &r.Ready); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
