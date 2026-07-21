// Package db reads DESIRED state from the shared `propertyteam` MySQL — the three
// host tables all stacks (pc/la/go) share: website_hosts (tenant), platform_hosts
// (system), platform_redirect_hosts (global redirect).
//
// This is a READ-ONLY conformer of that pc-owned schema: it SELECTs, never
// migrates. It reads ALL rows including inactive/soft-deleted ones, because a row
// that transitioned to inactive/soft-deleted is how the reconcile core learns a
// previously-rendered file should now be removed (vs an orphan file with no row).
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Row is one host row, normalized across the three tables into a common shape.
type Row struct {
	ID          int64  // primary key
	Table       string // "website_hosts" | "platform_hosts" | "platform_redirect_hosts"
	Host        string
	ServerStack string // website/platform only ("" for redirect)
	Target      string // platform: upstream host:port; redirect: URL ("" for website)
	Code        int    // redirect only
	IsActive    bool   // is_active = 1
	SoftDeleted bool   // deleted_at IS NOT NULL
}

// Desired reports whether a row is rendered to a file: active AND not soft-deleted.
func (r Row) Desired() bool { return r.IsActive && !r.SoftDeleted }

// Snapshot is the full read of all three tables at one instant.
type Snapshot struct {
	Rows          []Row
	ReadAt        time.Time      // when the snapshot was taken (caller-stamped)
	Sources       map[string]int // table -> row count read, for reporting
	MissingTables []string       // tables absent (pre-migration) — read as zero rows, not fatal
}

// DB is a thin read-only handle to the shared propertyteam MySQL.
type DB struct {
	sql *sql.DB
}

// Open connects using a Go MySQL DSN. It pings to fail fast on a bad
// DSN/credentials. The pool is kept small — short, infrequent read bursts.
func Open(ctx context.Context, dsn string) (*DB, error) {
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return &DB{sql: sqlDB}, nil
}

// Close releases the pool.
func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

// ReadSnapshot reads every row of the three tables (active, inactive, and
// soft-deleted). A MISSING table (e.g. platform_redirect_hosts not yet migrated)
// is tolerated: it reads as zero rows and is noted, rather than failing the read
// — degrade gracefully pre-migration, never wipe. Any OTHER error (auth,
// connectivity, a real query failure) is fatal, so a transient DB fault never
// masquerades as "the tables are empty".
func (d *DB) ReadSnapshot(ctx context.Context) (Snapshot, error) {
	snap := Snapshot{Sources: map[string]int{}}

	for _, tbl := range []struct {
		name string
		read func(context.Context) ([]Row, error)
	}{
		{"website_hosts", d.readWebsiteHosts},
		{"platform_hosts", d.readPlatformHosts},
		{"platform_redirect_hosts", d.readRedirectHosts},
	} {
		rows, err := tbl.read(ctx)
		if err != nil {
			if isMissingTable(err) {
				snap.MissingTables = append(snap.MissingTables, tbl.name)
				snap.Sources[tbl.name] = 0
				continue
			}
			return Snapshot{}, err
		}
		snap.Rows = append(snap.Rows, rows...)
		snap.Sources[tbl.name] = len(rows)
	}

	return snap, nil
}

// isMissingTable reports whether err is MySQL error 1146 (table doesn't exist).
func isMissingTable(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1146
}

// readWebsiteHosts reads the tenant host→stack pivot. website_hosts is LEAN in the
// current propertyteam schema (MyApp\Model\WebsiteHosts extends ModelBase): it has
// NO is_active and NO deleted_at — a row is "desired" simply by existing (removal
// is a hard delete so a freed host can be reused immediately). So every row read is
// active and non-deleted; the upstream is derived from server_stack, not a column.
func (d *DB) readWebsiteHosts(ctx context.Context) ([]Row, error) {
	const q = `SELECT id, host, server_stack FROM website_hosts`
	rows, err := d.sql.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("read website_hosts: %w", err)
	}
	defer rows.Close()

	var out []Row
	for rows.Next() {
		var id int64
		var host, stack string
		if err := rows.Scan(&id, &host, &stack); err != nil {
			return nil, fmt.Errorf("scan website_hosts: %w", err)
		}
		out = append(out, Row{
			ID: id, Table: "website_hosts", Host: host, ServerStack: stack,
			IsActive: true, SoftDeleted: false,
		})
	}
	return out, rows.Err()
}

func (d *DB) readPlatformHosts(ctx context.Context) ([]Row, error) {
	const q = `SELECT id, host, server_stack, target, is_active, deleted_at FROM platform_hosts`
	rows, err := d.sql.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("read platform_hosts: %w", err)
	}
	defer rows.Close()

	var out []Row
	for rows.Next() {
		var id int64
		var host, stack, target string
		var active int
		var deletedAt sql.NullTime
		if err := rows.Scan(&id, &host, &stack, &target, &active, &deletedAt); err != nil {
			return nil, fmt.Errorf("scan platform_hosts: %w", err)
		}
		out = append(out, Row{
			ID: id, Table: "platform_hosts", Host: host, ServerStack: stack, Target: target,
			IsActive: active != 0, SoftDeleted: deletedAt.Valid,
		})
	}
	return out, rows.Err()
}

func (d *DB) readRedirectHosts(ctx context.Context) ([]Row, error) {
	const q = `SELECT id, host, target, redirect_code, is_active, deleted_at FROM platform_redirect_hosts`
	rows, err := d.sql.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("read platform_redirect_hosts: %w", err)
	}
	defer rows.Close()

	var out []Row
	for rows.Next() {
		var id int64
		var host, target string
		var code, active int
		var deletedAt sql.NullTime
		if err := rows.Scan(&id, &host, &target, &code, &active, &deletedAt); err != nil {
			return nil, fmt.Errorf("scan platform_redirect_hosts: %w", err)
		}
		out = append(out, Row{
			ID: id, Table: "platform_redirect_hosts", Host: host, Target: target, Code: code,
			IsActive: active != 0, SoftDeleted: deletedAt.Valid,
		})
	}
	return out, rows.Err()
}
