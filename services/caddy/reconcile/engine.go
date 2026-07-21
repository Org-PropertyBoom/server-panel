package reconcile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ppt/server-panel/services/caddy/config"
	"ppt/server-panel/services/caddy/db"
	"ppt/server-panel/services/caddy/render"
	"ppt/server-panel/services/caddy/vhostfs"
)

// Adapter validates a Caddyfile by adapting it to JSON (satisfied by
// caddyctl.Adapter). Returning an error is the abort-on-invalid gate — the engine
// then does NOT reload.
type Adapter interface {
	Adapt(caddyfile []byte, filename string) (json []byte, warnings []string, err error)
}

// Reloader applies adapted JSON to Caddy and reads the current live config
// (satisfied by caddyctl.Client).
type Reloader interface {
	Load(ctx context.Context, adaptedJSON []byte) error
	CurrentConfig(ctx context.Context) ([]byte, error)
}

// SkipInfo is a JSON-friendly Skip.
type SkipInfo struct {
	Table  string `json:"table"`
	Host   string `json:"host"`
	Reason string `json:"reason"`
}

// Result is the truthful outcome of one reconcile — returned verbatim so a
// calling dashboard NEVER shows a fake success (the flash-wipe lesson). Error is
// non-empty iff the reconcile could not complete a validated reload.
type Result struct {
	Reloaded          bool           `json:"reloaded"`
	Written           []string       `json:"written"`
	Removed           []string       `json:"removed"`
	RemovesSuppressed []string       `json:"removes_suppressed,omitempty"`
	Orphans           []string       `json:"orphans"`
	Skips             []SkipInfo     `json:"skips,omitempty"`
	AdaptWarnings     []string       `json:"adapt_warnings,omitempty"`
	Sources           map[string]int `json:"sources,omitempty"`
	MissingTables     []string       `json:"missing_tables,omitempty"`
	BackupPath        string         `json:"backup_path,omitempty"`
	Error             string         `json:"error,omitempty"`
	DurationMS        int64          `json:"duration_ms"`
}

// Engine applies reconcile plans: it is the single serialized owner of the folder
// + reload. One reconcile runs at a time (mu). DryRun is read-only and needs no
// Adapter/Reloader (a Phase-1 engine may pass nil for those).
type Engine struct {
	cfg      config.Config
	dir      vhostfs.Dir
	adapter  Adapter
	reloader Reloader

	mu        sync.Mutex // serializes reconciles (one at a time)
	firstDone bool       // first reconcile completed → removals now permitted
	now       func() time.Time
}

// NewEngine constructs an Engine. Pass nil adapter/reloader for a read-only
// (DryRun-only) engine.
func NewEngine(cfg config.Config, adapter Adapter, reloader Reloader) *Engine {
	return &Engine{
		cfg:      cfg,
		dir:      vhostfs.New(cfg.VhostsDir),
		adapter:  adapter,
		reloader: reloader,
		now:      time.Now,
	}
}

// Reconcile runs one full pass: plan → atomic writes/removes → adapt (validate) →
// backup prior config → reload. Serialized. The Result is always populated
// (truthful); err mirrors Result.Error.
//
// SAFETY: on the FIRST reconcile of this process, no files are removed — removals
// are reported in RemovesSuppressed and applied only on subsequent passes (the
// anti-second-outage rule). Validation failure ABORTS before reload.
func (e *Engine) Reconcile(ctx context.Context, snap db.Snapshot) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	start := e.now()
	res := Result{Sources: snap.Sources, MissingTables: snap.MissingTables}

	if e.adapter == nil || e.reloader == nil {
		res.Error = "reconcile: engine has no adapter/reloader (read-only)"
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	if err := e.dir.Ensure(); err != nil {
		res.Error = "vhosts dir not usable: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	names, err := e.dir.ListNames()
	if err != nil {
		res.Error = "list folder: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	plan := BuildPlan(e.cfg, snap, names)
	res.Orphans = plan.Orphans
	for _, s := range plan.Skips {
		res.Skips = append(res.Skips, SkipInfo(s))
	}

	// Writes: atomic, idempotent. Report only files that actually changed.
	for _, w := range plan.Writes {
		changed, werr := e.dir.Write(w.Name, w.Contents)
		if werr != nil {
			res.Error = "write " + w.Name + ": " + werr.Error()
			res.DurationMS = e.since(start)
			return res, errors.New(res.Error)
		}
		if changed {
			res.Written = append(res.Written, w.Name)
		}
	}

	// Removes: suppressed on the very first pass (no prior state).
	if e.firstDone {
		for _, name := range plan.Removes {
			removed, rerr := e.dir.Remove(name)
			if rerr != nil {
				res.Error = "remove " + name + ": " + rerr.Error()
				res.DurationMS = e.since(start)
				return res, errors.New(res.Error)
			}
			if removed {
				res.Removed = append(res.Removed, name)
			}
		}
	} else if len(plan.Removes) > 0 {
		res.RemovesSuppressed = plan.Removes
		log.Printf("reconcile: first pass — suppressing %d removal(s), applying on next pass: %s",
			len(plan.Removes), strings.Join(plan.Removes, ", "))
	}

	// Validate: adapt the WHOLE main Caddyfile (it imports the folder we just wrote).
	// Abort before reload on any adapt error.
	mainBytes, err := os.ReadFile(e.cfg.MainCaddyfile)
	if err != nil {
		res.Error = "read main Caddyfile: " + err.Error()
		res.DurationMS = e.since(start)
		e.firstDone = true
		return res, errors.New(res.Error)
	}
	adapted, warnings, err := e.adapter.Adapt(mainBytes, e.cfg.MainCaddyfile)
	if err != nil {
		// The abort-on-invalid gate: files are on disk but Caddy is untouched.
		res.Error = "validate (adapt): " + err.Error()
		res.AdaptWarnings = warnings
		res.DurationMS = e.since(start)
		e.firstDone = true
		return res, errors.New(res.Error)
	}
	res.AdaptWarnings = warnings
	if err := e.assertDashboardPresent(adapted); err != nil {
		res.Error = err.Error()
		res.DurationMS = e.since(start)
		e.firstDone = true
		return res, err
	}

	// Backup the PRIOR live config before we replace it (best-effort).
	if path, berr := e.backupPrior(ctx); berr != nil {
		log.Printf("reconcile: prior-config backup failed (continuing): %v", berr)
	} else {
		res.BackupPath = path
	}

	// Reload: POST the adapted JSON. On failure, Caddy keeps its old config.
	if err := e.reloader.Load(ctx, adapted); err != nil {
		res.Error = "reload: " + err.Error()
		res.DurationMS = e.since(start)
		e.firstDone = true
		return res, errors.New(res.Error)
	}
	res.Reloaded = true

	e.firstDone = true
	res.DurationMS = e.since(start)
	return res, nil
}

// ReloadOnly re-validates and reloads the CURRENT folder state without reading
// the DB or mutating any file — the "force a validated reload" button. It still
// goes through the adapt gate and the prior-config backup.
func (e *Engine) ReloadOnly(ctx context.Context) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	start := e.now()
	res := Result{}

	if e.adapter == nil || e.reloader == nil {
		res.Error = "reload: engine has no adapter/reloader (read-only)"
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}

	mainBytes, err := os.ReadFile(e.cfg.MainCaddyfile)
	if err != nil {
		res.Error = "read main Caddyfile: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	adapted, warnings, err := e.adapter.Adapt(mainBytes, e.cfg.MainCaddyfile)
	res.AdaptWarnings = warnings
	if err != nil {
		res.Error = "validate (adapt): " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	if err := e.assertDashboardPresent(adapted); err != nil {
		res.Error = err.Error()
		res.DurationMS = e.since(start)
		return res, err
	}
	if path, berr := e.backupPrior(ctx); berr != nil {
		log.Printf("reload: prior-config backup failed (continuing): %v", berr)
	} else {
		res.BackupPath = path
	}
	if err := e.reloader.Load(ctx, adapted); err != nil {
		res.Error = "reload: " + err.Error()
		res.DurationMS = e.since(start)
		return res, errors.New(res.Error)
	}
	res.Reloaded = true
	res.DurationMS = e.since(start)
	return res, nil
}

// assertDashboardPresent refuses to reload any adapted config that does not
// contain the dashboard domain — the 2026-07-11 outage signature (the adapted
// config collapsed because the import read nothing). Inverted here into a hard
// invariant: the dashboard domain is a STATIC block in the main Caddyfile, so it
// must ALWAYS survive adapt; if it doesn't, the main Caddyfile itself is broken.
func (e *Engine) assertDashboardPresent(adapted []byte) error {
	dash := strings.ToLower(strings.TrimSpace(e.cfg.DashboardDomain))
	if dash == "" {
		return nil
	}
	if !bytes.Contains(bytes.ToLower(adapted), []byte(dash)) {
		return fmt.Errorf("validate: adapted config is missing the dashboard domain %q — REFUSING to reload (the main Caddyfile's static block is broken; this is the outage signature)", dash)
	}
	return nil
}

func (e *Engine) since(start time.Time) int64 {
	return e.now().Sub(start).Milliseconds()
}

// RemoveFile deletes one `<host>.caddy` file — the explicit operator-driven
// "prune this orphan" action. It REFUSES a protected file (dashboard/panel domain,
// or a wildcard). It does NOT reconcile; the caller reconciles after so the removal
// is validated + reloaded. Returns whether a file was removed.
func (e *Engine) RemoveFile(name string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, h := range e.cfg.ProtectedHosts() {
		if name == render.FileName(h) {
			return false, fmt.Errorf("refusing to remove protected domain file %q", name)
		}
	}
	if strings.HasPrefix(name, "wildcard_") {
		return false, fmt.Errorf("refusing to remove reserved wildcard file %q", name)
	}
	return e.dir.Remove(name)
}

// FileState is one folder file for the drift view.
type FileState struct {
	Name     string `json:"name"`
	Size     int    `json:"size"`
	Contents string `json:"contents"`
}

// HostRow is one host as the cockpit renders it: its class, upstream, and the
// status a reconcile WOULD produce (never applied by DryRun).
type HostRow struct {
	Hostname string `json:"hostname"`
	Kind     string `json:"kind"`               // "tenant" | "system" | "redirect" | "orphan"
	Stack    string `json:"stack,omitempty"`    // server_stack for tenant/system
	Upstream string `json:"upstream,omitempty"` // reverse_proxy upstream or redirect URL
	Status   string `json:"status"`             // "in_sync" | "will_write" | "will_remove" | "orphan"
}

// DryRunResult is the read-only drift view: what a reconcile WOULD do, computed
// without touching anything.
type DryRunResult struct {
	VhostsDir     string         `json:"vhosts_dir"`
	Files         []FileState    `json:"files"`
	Hosts         []HostRow      `json:"hosts"`
	DesiredCount  int            `json:"desired_count"`
	WouldWrite    []string       `json:"would_write"`
	WouldRemove   []string       `json:"would_remove"`
	Orphans       []string       `json:"orphans"`
	Skips         []SkipInfo     `json:"skips,omitempty"`
	Sources       map[string]int `json:"sources,omitempty"`
	MissingTables []string       `json:"missing_tables,omitempty"`
	InSync        bool           `json:"in_sync"`
	FirstPassDone bool           `json:"first_pass_done"`
}

// DryRun computes the plan for a snapshot against the current folder WITHOUT
// applying anything.
func (e *Engine) DryRun(snap db.Snapshot) (DryRunResult, error) {
	if err := e.dir.Ensure(); err != nil {
		return DryRunResult{}, err
	}
	files, err := e.dir.List()
	if err != nil {
		return DryRunResult{}, err
	}
	onDisk := make(map[string]string, len(files))
	names := make([]string, 0, len(files))
	for _, f := range files {
		onDisk[f.Name] = f.Contents
		names = append(names, f.Name)
	}
	plan := BuildPlan(e.cfg, snap, names)

	var wouldWrite []string
	for _, w := range plan.Writes {
		if cur, ok := onDisk[w.Name]; !ok || cur != w.Contents {
			wouldWrite = append(wouldWrite, w.Name)
		}
	}
	sort.Strings(wouldWrite)

	out := DryRunResult{
		VhostsDir:     e.cfg.VhostsDir,
		DesiredCount:  len(plan.Writes),
		WouldWrite:    wouldWrite,
		WouldRemove:   plan.Removes,
		Orphans:       plan.Orphans,
		Sources:       snap.Sources,
		MissingTables: snap.MissingTables,
		InSync:        len(wouldWrite) == 0 && len(plan.Removes) == 0,
		FirstPassDone: e.firstPassDone(),
	}
	for _, s := range plan.Skips {
		out.Skips = append(out.Skips, SkipInfo(s))
	}
	for _, f := range files {
		out.Files = append(out.Files, FileState{Name: f.Name, Size: len(f.Contents), Contents: f.Contents})
	}

	// Per-host cockpit rows: desired hosts (in_sync/will_write), plus removals and
	// orphans (never applied here — just reported).
	for _, w := range plan.Writes {
		status := "in_sync"
		if cur, ok := onDisk[w.Name]; !ok || cur != w.Contents {
			status = "will_write"
		}
		out.Hosts = append(out.Hosts, HostRow{
			Hostname: w.Host, Kind: w.Kind, Stack: w.Stack, Upstream: w.Upstream, Status: status,
		})
	}
	for _, name := range plan.Removes {
		out.Hosts = append(out.Hosts, HostRow{Hostname: render.HostFromFileName(name), Status: "will_remove"})
	}
	for _, name := range plan.Orphans {
		out.Hosts = append(out.Hosts, HostRow{Hostname: render.HostFromFileName(name), Kind: "orphan", Status: "orphan"})
	}
	return out, nil
}

func (e *Engine) firstPassDone() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.firstDone
}

// backupPrior fetches the current live config and writes it to the backup dir as
// prior-<timestamp>.json plus prior-latest.json (the one-command rollback target).
func (e *Engine) backupPrior(ctx context.Context) (string, error) {
	if strings.TrimSpace(e.cfg.BackupDir) == "" {
		return "", nil
	}
	prior, err := e.reloader.CurrentConfig(ctx)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(e.cfg.BackupDir, 0o750); err != nil {
		return "", fmt.Errorf("mkdir backup dir: %w", err)
	}
	ts := e.now().UTC().Format("20060102T150405Z")
	path := filepath.Join(e.cfg.BackupDir, "prior-"+ts+".json")
	if err := os.WriteFile(path, prior, 0o640); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}
	_ = os.WriteFile(filepath.Join(e.cfg.BackupDir, "prior-latest.json"), prior, 0o640)
	e.pruneBackups()
	return path, nil
}

const backupKeep = 10

// pruneBackups keeps only the most recent backupKeep prior-*.json snapshots.
func (e *Engine) pruneBackups() {
	entries, err := os.ReadDir(e.cfg.BackupDir)
	if err != nil {
		return
	}
	var snaps []string
	for _, en := range entries {
		n := en.Name()
		if strings.HasPrefix(n, "prior-") && strings.HasSuffix(n, ".json") && n != "prior-latest.json" {
			snaps = append(snaps, n)
		}
	}
	if len(snaps) <= backupKeep {
		return
	}
	sort.Strings(snaps)
	for _, old := range snaps[:len(snaps)-backupKeep] {
		_ = os.Remove(filepath.Join(e.cfg.BackupDir, old))
	}
}
