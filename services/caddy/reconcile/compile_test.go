package reconcile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ppt/server-panel/services/caddy/config"
	"ppt/server-panel/services/caddy/db"
)

// fakePersistCaddy is a fakeCaddy that ALSO implements ConfigPersister, so the
// engine takes the compile-then-reload-from-file path instead of the admin-API
// fallback.
type fakePersistCaddy struct {
	fakeCaddy
	persistErr   error
	reloadErr    error
	persisted    []byte
	persistPath  string
	persists     int
	fileReloads  int
	reloadedFrom string
	// order records the call sequence so the persist-BEFORE-reload ordering is
	// asserted, not assumed.
	order []string
}

func (f *fakePersistCaddy) PersistConfig(path string, adaptedJSON []byte) error {
	f.order = append(f.order, "persist")
	if f.persistErr != nil {
		return f.persistErr
	}
	f.persists++
	f.persistPath = path
	f.persisted = append([]byte(nil), adaptedJSON...)
	return nil
}

func (f *fakePersistCaddy) ReloadFromFile(ctx context.Context, path string) error {
	f.order = append(f.order, "reload")
	if f.reloadErr != nil {
		return f.reloadErr
	}
	f.fileReloads++
	f.reloadedFrom = path
	return nil
}

func compileCfg(t *testing.T) (config.Config, string) {
	t.Helper()
	cfg := engineCfg(t)
	compiled := filepath.Join(t.TempDir(), "caddy.json")
	cfg.CompiledConfigPath = compiled
	return cfg, compiled
}

func rowsOne() db.Snapshot {
	return db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "a.com", ServerStack: "golang", IsActive: true},
	}}
}

// TestReconcile_CompilesThenReloadsFromFile is the core of the 2026-09-07 fix:
// an apply must persist the config and reload OUT OF the persisted file, never
// push it to the admin API only.
func TestReconcile_CompilesThenReloadsFromFile(t *testing.T) {
	cfg, compiled := compileCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakePersistCaddy{}
	e := NewEngine(cfg, fc, fc)

	res, err := e.Reconcile(context.Background(), rowsOne())
	if err != nil {
		t.Fatalf("Reconcile: %v (result error %q)", err, res.Error)
	}
	if !res.Reloaded {
		t.Fatal("expected Reloaded=true")
	}
	if fc.persists != 1 {
		t.Errorf("persists = %d, want 1", fc.persists)
	}
	if fc.fileReloads != 1 {
		t.Errorf("fileReloads = %d, want 1", fc.fileReloads)
	}
	// The memory-only admin push is what caused the outage — it must not happen.
	if fc.loads != 0 {
		t.Errorf("admin-API Load called %d times; the compiled path must not use it", fc.loads)
	}
	if fc.persistPath != compiled || fc.reloadedFrom != compiled {
		t.Errorf("persisted to %q and reloaded from %q, want both %q", fc.persistPath, fc.reloadedFrom, compiled)
	}
	if res.CompiledPath != compiled {
		t.Errorf("res.CompiledPath = %q, want %q", res.CompiledPath, compiled)
	}
	// PERSIST MUST COME FIRST. Reloading first would leave a window in which the
	// running config exists nowhere on disk — the exact shape of the outage.
	if len(fc.order) != 2 || fc.order[0] != "persist" || fc.order[1] != "reload" {
		t.Errorf("call order = %v, want [persist reload]", fc.order)
	}
	if !json.Valid(fc.persisted) {
		t.Errorf("persisted bytes are not valid JSON: %q", fc.persisted)
	}
}

// TestForceReload_AlsoRecompiles — "fix it with Force reload" must refresh the
// BOOT config too, or the repair silently reverts at the next restart.
func TestForceReload_AlsoRecompiles(t *testing.T) {
	cfg, compiled := compileCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakePersistCaddy{}
	e := NewEngine(cfg, fc, fc)

	res, err := e.ReloadOnly(context.Background())
	if err != nil {
		t.Fatalf("ReloadOnly: %v (result error %q)", err, res.Error)
	}
	if fc.persists != 1 || fc.fileReloads != 1 {
		t.Errorf("persists=%d fileReloads=%d, want 1/1", fc.persists, fc.fileReloads)
	}
	if fc.loads != 0 {
		t.Errorf("admin-API Load called %d times on force reload", fc.loads)
	}
	if res.CompiledPath != compiled {
		t.Errorf("CompiledPath = %q, want %q", res.CompiledPath, compiled)
	}
}

// TestReconcile_PersistFailureAbortsBeforeReload — if we cannot write the boot
// config, we must NOT change the running one, or running and boot diverge again.
func TestReconcile_PersistFailureAbortsBeforeReload(t *testing.T) {
	cfg, _ := compileCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakePersistCaddy{persistErr: errors.New("read-only filesystem")}
	e := NewEngine(cfg, fc, fc)

	res, err := e.Reconcile(context.Background(), rowsOne())
	if err == nil {
		t.Fatal("expected an error when the compiled config cannot be written")
	}
	if res.Reloaded {
		t.Error("Reloaded must be false when persisting failed")
	}
	if fc.fileReloads != 0 || fc.loads != 0 {
		t.Errorf("nothing may be applied after a persist failure (fileReloads=%d loads=%d)", fc.fileReloads, fc.loads)
	}
	if res.Error == "" {
		t.Error("Result.Error must carry the failure for the panel to surface")
	}
}

// TestReconcile_ReloadFailureIsReportedButFilePersists documents the deliberate
// asymmetry: on a failed reload the file is NEWER than the running config. That
// is the safe side — a later cold start lands on the intended config.
func TestReconcile_ReloadFailureIsReportedButFilePersists(t *testing.T) {
	cfg, compiled := compileCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakePersistCaddy{reloadErr: errors.New("admin endpoint refused")}
	e := NewEngine(cfg, fc, fc)

	res, err := e.Reconcile(context.Background(), rowsOne())
	if err == nil {
		t.Fatal("expected an error when the reload fails")
	}
	if res.Reloaded {
		t.Error("Reloaded must be false when the reload failed — never a fake success")
	}
	if fc.persists != 1 {
		t.Errorf("persists = %d, want 1 (the file is written before the reload)", fc.persists)
	}
	if res.CompiledPath != compiled {
		t.Errorf("CompiledPath = %q, want it reported even on reload failure", res.CompiledPath)
	}
}

// TestReconcile_FallsBackToAdminLoadWhenCompilingDisabled keeps off-host and
// read-only engines working: no compiled path => the previous admin-API behaviour.
func TestReconcile_FallsBackToAdminLoadWhenCompilingDisabled(t *testing.T) {
	cfg := engineCfg(t)
	cfg.CompiledConfigPath = "" // explicitly disabled
	mkVhosts(t, cfg, nil)
	fc := &fakePersistCaddy{}
	e := NewEngine(cfg, fc, fc)

	res, err := e.Reconcile(context.Background(), rowsOne())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if fc.loads != 1 {
		t.Errorf("admin Load called %d times, want 1 (fallback path)", fc.loads)
	}
	if fc.persists != 0 || fc.fileReloads != 0 {
		t.Errorf("must not compile when disabled (persists=%d fileReloads=%d)", fc.persists, fc.fileReloads)
	}
	if res.CompiledPath != "" {
		t.Errorf("CompiledPath = %q, want empty when compiling is off", res.CompiledPath)
	}
}

// TestPublish_RejectsNonJSON — the file systemd boots from must never receive
// something unparseable, even if adapt somehow returned it.
func TestPublish_RejectsNonJSON(t *testing.T) {
	cfg, _ := compileCfg(t)
	fc := &fakePersistCaddy{}
	e := NewEngine(cfg, fc, fc)

	path, err := e.publish(context.Background(), []byte("this is definitely not json, but it is long enough to pass the length gate"))
	if err == nil {
		t.Fatal("expected an error for non-JSON adapted output")
	}
	if path != "" || fc.persists != 0 || fc.fileReloads != 0 {
		t.Errorf("nothing may be persisted or reloaded (path=%q persists=%d reloads=%d)", path, fc.persists, fc.fileReloads)
	}
}

// TestCompiledPathNotCreatedOnAdaptFailure — an invalid adapt aborts the whole
// apply, leaving no partial write for systemd to boot from.
func TestCompiledPathNotCreatedOnAdaptFailure(t *testing.T) {
	cfg, compiled := compileCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakePersistCaddy{}
	fc.adaptErr = errors.New("caddyfile syntax error")
	e := NewEngine(cfg, fc, fc)

	if _, err := e.Reconcile(context.Background(), rowsOne()); err == nil {
		t.Fatal("expected the adapt failure to abort the apply")
	}
	if _, err := os.Stat(compiled); !os.IsNotExist(err) {
		t.Errorf("compiled config must not exist after an adapt failure: %v", err)
	}
	if fc.persists != 0 {
		t.Errorf("persists = %d, want 0 — abort happens before compiling", fc.persists)
	}
}
