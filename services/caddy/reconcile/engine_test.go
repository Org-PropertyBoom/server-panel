package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	"ppt/server-panel/services/caddy/config"
	"ppt/server-panel/services/caddy/db"
)

func TestDryRun_ReportsDriftAndOrphans(t *testing.T) {
	dir := t.TempDir()
	// On disk: an orphan (no DB row) + a managed file whose bytes are out of date.
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
	e := NewEngine(cfg)

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
	e := NewEngine(cfg)
	snap := db.Snapshot{Rows: []db.Row{{Table: "website_hosts", Host: "tenant.com", ServerStack: "golang", IsActive: true}}}
	res, err := e.DryRun(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !res.InSync {
		t.Errorf("should be in sync; WouldWrite=%v WouldRemove=%v", res.WouldWrite, res.WouldRemove)
	}
}

func TestDryRun_MissingDirErrors(t *testing.T) {
	e := NewEngine(config.Config{VhostsDir: filepath.Join(t.TempDir(), "does-not-exist")})
	if _, err := e.DryRun(db.Snapshot{}); err == nil {
		t.Error("DryRun on a missing vhosts dir should error")
	}
}
