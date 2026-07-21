package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// The engine writes ONLY the two platform-owned tables — platform_hosts (system
// vhosts) and platform_redirect_hosts (edge redirects) — from its own management
// UI. Tenant website_hosts stays owned by the stack apps (read-only here).
// created_by/updated_by are left NULL: the panel is a system actor. Timestamps
// use the DB clock (NOW()).

// ErrDuplicateHost is returned when a create/update would violate the table's
// UNIQUE(host) / UNIQUE(server_stack, host) constraint.
var ErrDuplicateHost = errors.New("a row with that host already exists")

// ErrNotFound is returned when an update/delete targets a missing (or already
// soft-deleted) id.
var ErrNotFound = errors.New("row not found (or already deleted)")

// SystemHostInput is the create/update payload for a platform_hosts row.
type SystemHostInput struct {
	Host        string
	ServerStack string // phalcon | laravel | golang
	Target      string // upstream host:port
	IsActive    bool
}

// RedirectInput is the create/update payload for a platform_redirect_hosts row.
type RedirectInput struct {
	Host     string
	Target   string // destination URL
	Code     int    // 301 | 302
	IsActive bool
}

// CreateSystemHost inserts a platform_hosts row and returns its new id.
func (d *DB) CreateSystemHost(ctx context.Context, in SystemHostInput) (int64, error) {
	const q = `INSERT INTO platform_hosts (host, server_stack, target, is_active, created_at, updated_at)
	           VALUES (?, ?, ?, ?, NOW(), NOW())`
	res, err := d.sql.ExecContext(ctx, q, in.Host, in.ServerStack, in.Target, boolToInt(in.IsActive))
	if err != nil {
		return 0, wrapWrite("create system host", err)
	}
	return res.LastInsertId()
}

// UpdateSystemHost updates a non-deleted platform_hosts row by id.
func (d *DB) UpdateSystemHost(ctx context.Context, id int64, in SystemHostInput) error {
	const q = `UPDATE platform_hosts SET host=?, server_stack=?, target=?, is_active=?, updated_at=NOW()
	           WHERE id=? AND deleted_at IS NULL`
	return d.execAffecting(ctx, "update system host", q, in.Host, in.ServerStack, in.Target, boolToInt(in.IsActive), id)
}

// SetSystemHostActive toggles is_active on a non-deleted platform_hosts row.
func (d *DB) SetSystemHostActive(ctx context.Context, id int64, active bool) error {
	const q = `UPDATE platform_hosts SET is_active=?, updated_at=NOW() WHERE id=? AND deleted_at IS NULL`
	return d.execAffecting(ctx, "toggle system host", q, boolToInt(active), id)
}

// DeleteSystemHost soft-deletes a platform_hosts row (sets deleted_at). The
// reconcile core then removes its rendered file on the next (non-first) pass.
func (d *DB) DeleteSystemHost(ctx context.Context, id int64) error {
	const q = `UPDATE platform_hosts SET deleted_at=NOW(), updated_at=NOW() WHERE id=? AND deleted_at IS NULL`
	return d.execAffecting(ctx, "delete system host", q, id)
}

// CreateRedirect inserts a platform_redirect_hosts row and returns its new id.
func (d *DB) CreateRedirect(ctx context.Context, in RedirectInput) (int64, error) {
	const q = `INSERT INTO platform_redirect_hosts (host, target, redirect_code, is_active, created_at, updated_at)
	           VALUES (?, ?, ?, ?, NOW(), NOW())`
	res, err := d.sql.ExecContext(ctx, q, in.Host, in.Target, in.Code, boolToInt(in.IsActive))
	if err != nil {
		return 0, wrapWrite("create redirect", err)
	}
	return res.LastInsertId()
}

// UpdateRedirect updates a non-deleted platform_redirect_hosts row by id.
func (d *DB) UpdateRedirect(ctx context.Context, id int64, in RedirectInput) error {
	const q = `UPDATE platform_redirect_hosts SET host=?, target=?, redirect_code=?, is_active=?, updated_at=NOW()
	           WHERE id=? AND deleted_at IS NULL`
	return d.execAffecting(ctx, "update redirect", q, in.Host, in.Target, in.Code, boolToInt(in.IsActive), id)
}

// SetRedirectActive toggles is_active on a non-deleted redirect row.
func (d *DB) SetRedirectActive(ctx context.Context, id int64, active bool) error {
	const q = `UPDATE platform_redirect_hosts SET is_active=?, updated_at=NOW() WHERE id=? AND deleted_at IS NULL`
	return d.execAffecting(ctx, "toggle redirect", q, boolToInt(active), id)
}

// DeleteRedirect soft-deletes a redirect row.
func (d *DB) DeleteRedirect(ctx context.Context, id int64) error {
	const q = `UPDATE platform_redirect_hosts SET deleted_at=NOW(), updated_at=NOW() WHERE id=? AND deleted_at IS NULL`
	return d.execAffecting(ctx, "delete redirect", q, id)
}

// execAffecting runs an UPDATE and maps 0-rows-affected to ErrNotFound and a
// unique violation to ErrDuplicateHost.
func (d *DB) execAffecting(ctx context.Context, what, q string, args ...any) error {
	res, err := d.sql.ExecContext(ctx, q, args...)
	if err != nil {
		return wrapWrite(what, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	if n == 0 {
		return fmt.Errorf("%s: %w", what, ErrNotFound)
	}
	return nil
}

// wrapWrite maps a MySQL duplicate-key (1062) to ErrDuplicateHost, else wraps.
func wrapWrite(what string, err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) && me.Number == 1062 {
		return fmt.Errorf("%s: %w", what, ErrDuplicateHost)
	}
	return fmt.Errorf("%s: %w", what, err)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
