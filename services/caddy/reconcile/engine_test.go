package reconcile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ppt/server-panel/services/caddy/config"
	"ppt/server-panel/services/caddy/db"
)

// fakeCaddy satisfies both Adapter and Reloader. It records loads and can be told
// to fail adapt (simulating an invalid config -> abort gate).
type fakeCaddy struct {
	adaptErr    error
	adaptNoDash bool
	loadErr     error
	loads       int
	lastJSON    []byte
	current     []byte
	currErr     error
}

func (f *fakeCaddy) Adapt(caddyfile []byte, filename string) ([]byte, []string, error) {
	if f.adaptErr != nil {
		return nil, nil, f.adaptErr
	}
	if f.adaptNoDash {
		// Simulate the outage signature: adapted config missing the dashboard domain.
		return []byte(`{"apps":{"http":{"servers":{"srv0":{"routes":[]}}}}}________`), nil, nil
	}
	return []byte(`{"apps":{"http":{"servers":{"srv0":{"routes":[{"match":[{"host":["app.propertyboom.co"]}]}]}}}}}`), nil, nil
}

func (f *fakeCaddy) Load(ctx context.Context, adaptedJSON []byte) error {
	if f.loadErr != nil {
		return f.loadErr
	}
	f.loads++
	f.lastJSON = adaptedJSON
	return nil
}

func (f *fakeCaddy) CurrentConfig(ctx context.Context) ([]byte, error) {
	if f.currErr != nil {
		return nil, f.currErr
	}
	if f.current == nil {
		return []byte(`{"prior":true}`), nil
	}
	return f.current, nil
}

func engineCfg(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	main := filepath.Join(dir, "Caddyfile")
	if err := os.WriteFile(main, []byte("app.propertyboom.co {\n\treverse_proxy 127.0.0.1:8002\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return config.Config{
		DashboardDomain: "app.propertyboom.co",
		VhostsDir:       filepath.Join(dir, "vhosts"),
		MainCaddyfile:   main,
		BackupDir:       filepath.Join(dir, "backups"),
		StackPorts:      map[string]string{"phalcon": "127.0.0.1:8002", "golang": "127.0.0.1:8005"},
	}
}

func mkVhosts(t *testing.T, cfg config.Config, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(cfg.VhostsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(cfg.VhostsDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEngine_HappyPath_WritesAdaptsBacksUpReloads(t *testing.T) {
	cfg := engineCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakeCaddy{}
	e := NewEngine(cfg, fc, fc)

	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "a.com", ServerStack: "golang", IsActive: true},
	}}
	res, err := e.Reconcile(context.Background(), snap)
	if err != nil {
		t.Fatalf("Reconcile: %v (%s)", err, res.Error)
	}
	if !res.Reloaded {
		t.Error("expected Reloaded=true")
	}
	if fc.loads != 1 {
		t.Errorf("expected 1 load, got %d", fc.loads)
	}
	if len(res.Written) != 1 || res.Written[0] != "a.com.caddy" {
		t.Errorf("Written = %v", res.Written)
	}
	b, _ := os.ReadFile(filepath.Join(cfg.VhostsDir, "a.com.caddy"))
	if string(b) != "a.com {\n    reverse_proxy 127.0.0.1:8005\n}\n" {
		t.Errorf("file contents = %q", b)
	}
	if res.BackupPath == "" {
		t.Error("expected a backup path")
	}
	if _, err := os.Stat(filepath.Join(cfg.BackupDir, "prior-latest.json")); err != nil {
		t.Errorf("prior-latest.json not written: %v", err)
	}
}

func TestEngine_FirstPassSuppressesRemoves_ThenApplies(t *testing.T) {
	cfg := engineCfg(t)
	mkVhosts(t, cfg, map[string]string{"gone.com.caddy": "gone.com {\n    reverse_proxy 127.0.0.1:8002\n}\n"})
	fc := &fakeCaddy{}
	e := NewEngine(cfg, fc, fc)

	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "gone.com", ServerStack: "phalcon", IsActive: false},
	}}

	res, err := e.Reconcile(context.Background(), snap)
	if err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	if len(res.Removed) != 0 {
		t.Errorf("first pass must remove nothing; got %v", res.Removed)
	}
	if len(res.RemovesSuppressed) != 1 || res.RemovesSuppressed[0] != "gone.com.caddy" {
		t.Errorf("first pass should report suppressed removal; got %v", res.RemovesSuppressed)
	}
	if _, err := os.Stat(filepath.Join(cfg.VhostsDir, "gone.com.caddy")); err != nil {
		t.Errorf("file must still exist after first pass: %v", err)
	}

	res, err = e.Reconcile(context.Background(), snap)
	if err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "gone.com.caddy" {
		t.Errorf("second pass should remove; got %v", res.Removed)
	}
	if _, err := os.Stat(filepath.Join(cfg.VhostsDir, "gone.com.caddy")); !os.IsNotExist(err) {
		t.Errorf("file should be gone after second pass")
	}
}

func TestEngine_AbortOnInvalid_DoesNotReload(t *testing.T) {
	cfg := engineCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakeCaddy{adaptErr: errors.New("adapt boom")}
	e := NewEngine(cfg, fc, fc)

	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "a.com", ServerStack: "golang", IsActive: true},
	}}
	res, err := e.Reconcile(context.Background(), snap)
	if err == nil {
		t.Fatal("expected an error when adapt fails")
	}
	if res.Reloaded {
		t.Error("must NOT reload when validation fails")
	}
	if fc.loads != 0 {
		t.Errorf("Load must not be called on adapt failure; got %d", fc.loads)
	}
	if res.Error == "" {
		t.Error("Result.Error should carry the truthful failure reason")
	}
	if len(res.Written) != 1 {
		t.Errorf("Written = %v", res.Written)
	}
}

func TestEngine_RefusesReloadIfDashboardDomainMissing(t *testing.T) {
	cfg := engineCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakeCaddy{adaptNoDash: true}
	e := NewEngine(cfg, fc, fc)

	res, err := e.Reconcile(context.Background(), db.Snapshot{})
	if err == nil {
		t.Fatal("must refuse to reload when the dashboard domain is absent from adapted output")
	}
	if res.Reloaded || fc.loads != 0 {
		t.Errorf("must NOT reload; reloaded=%v loads=%d", res.Reloaded, fc.loads)
	}
	if res.Error == "" {
		t.Error("Error should explain the missing-dashboard-domain refusal")
	}
}

func TestEngine_ReloadOnly_RefusesIfDashboardMissing(t *testing.T) {
	cfg := engineCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakeCaddy{adaptNoDash: true}
	e := NewEngine(cfg, fc, fc)
	res, err := e.ReloadOnly(context.Background())
	if err == nil || res.Reloaded || fc.loads != 0 {
		t.Errorf("ReloadOnly must also refuse; err=%v reloaded=%v loads=%d", err, res.Reloaded, fc.loads)
	}
}

func TestEngine_ReloadFailureSurfaced(t *testing.T) {
	cfg := engineCfg(t)
	mkVhosts(t, cfg, nil)
	fc := &fakeCaddy{loadErr: errors.New("admin down")}
	e := NewEngine(cfg, fc, fc)

	res, err := e.Reconcile(context.Background(), db.Snapshot{})
	if err == nil {
		t.Fatal("expected reload error")
	}
	if res.Reloaded {
		t.Error("Reloaded must be false on load failure")
	}
	if res.Error == "" {
		t.Error("Error must be populated")
	}
}

// --- read-only DryRun (Phase 1) — a nil adapter/reloader engine ---

func TestDryRun_ReportsDriftAndOrphans(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "orphan.com.caddy"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tenant.com.caddy"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		VhostsDir:       dir,
		DashboardDomain: "app.propertyboom.co",
		PanelDomain:     "cp.propertyweb.co",
		StackPorts:      map[string]string{"golang": "127.0.0.1:8005"},
	}
	e := NewEngine(cfg, nil, nil)

	snap := db.Snapshot{
		Rows:    []db.Row{{Table: "website_hosts", Host: "tenant.com", ServerStack: "golang", IsActive: true}},
		Sources: map[string]int{"website_hosts": 1},
	}

	res, err := e.DryRun(snap)
	if err != nil {
		t.Fatal(err)
	}
	if res.InSync {
		t.Error("folder differs from desired — must not report in-sync")
	}
	if len(res.WouldWrite) != 1 || res.WouldWrite[0] != "tenant.com.caddy" {
		t.Errorf("WouldWrite = %v, want [tenant.com.caddy]", res.WouldWrite)
	}
	if len(res.Orphans) != 1 || res.Orphans[0] != "orphan.com.caddy" {
		t.Errorf("Orphans = %v, want [orphan.com.caddy]", res.Orphans)
	}
	if len(res.WouldRemove) != 0 {
		t.Errorf("WouldRemove = %v, want none (orphans are never auto-removed)", res.WouldRemove)
	}
}

func TestDryRun_InSyncWhenFolderMatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(dir, "tenant.com.caddy"),
		[]byte("tenant.com {\n    reverse_proxy 127.0.0.1:8005\n}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		VhostsDir:       dir,
		DashboardDomain: "app.propertyboom.co",
		StackPorts:      map[string]string{"golang": "127.0.0.1:8005"},
	}
	e := NewEngine(cfg, nil, nil)
	snap := db.Snapshot{Rows: []db.Row{{Table: "website_hosts", Host: "tenant.com", ServerStack: "golang", IsActive: true}}}
	res, err := e.DryRun(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !res.InSync {
		t.Errorf("should be in sync; WouldWrite=%v WouldRemove=%v", res.WouldWrite, res.WouldRemove)
	}
}

func TestReconcile_RefusedWhenReadOnly(t *testing.T) {
	e := NewEngine(config.Config{VhostsDir: t.TempDir()}, nil, nil)
	if _, err := e.Reconcile(context.Background(), db.Snapshot{}); err == nil {
		t.Error("a nil-adapter engine must refuse to Reconcile")
	}
}

func TestRemoveFile_RefusesProtectedAndWildcard(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{VhostsDir: dir, DashboardDomain: "app.propertyboom.co", PanelDomain: "cp.propertyweb.co"}
	e := NewEngine(cfg, nil, nil)
	if _, err := e.RemoveFile("app.propertyboom.co.caddy"); err == nil {
		t.Error("must refuse to remove the dashboard-domain file")
	}
	if _, err := e.RemoveFile("cp.propertyweb.co.caddy"); err == nil {
		t.Error("must refuse to remove the panel-domain file")
	}
	if _, err := e.RemoveFile("wildcard_example.com.caddy"); err == nil {
		t.Error("must refuse to remove a wildcard file")
	}
}
