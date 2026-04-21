package switchboard

import (
	"context"
	"database/sql"
	"time"
)

type DB struct {
	db *sql.DB
}

func NewDB(db *sql.DB) *DB { return &DB{db: db} }

// LaunchedLoom records the parameters needed to re-launch a loom on restart.
type LaunchedLoom struct {
	ID        string
	Name      string
	Command   string
	WorkDir   string
	CreatedAt time.Time
}

func (d *DB) SaveLaunchedLoom(ctx context.Context, l LaunchedLoom) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO launched_looms (id, name, command, work_dir, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		l.ID, l.Name, l.Command, l.WorkDir, l.CreatedAt,
	)
	return err
}

func (d *DB) DeleteLaunchedLoom(ctx context.Context, id string) error {
	_, err := d.db.ExecContext(ctx,
		`DELETE FROM launched_looms WHERE id=?`, id,
	)
	return err
}

func (d *DB) ListLaunchedLooms(ctx context.Context) ([]LaunchedLoom, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, name, command, work_dir, created_at FROM launched_looms ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var looms []LaunchedLoom
	for rows.Next() {
		var l LaunchedLoom
		if err := rows.Scan(&l.ID, &l.Name, &l.Command, &l.WorkDir, &l.CreatedAt); err != nil {
			return nil, err
		}
		looms = append(looms, l)
	}
	return looms, rows.Err()
}
