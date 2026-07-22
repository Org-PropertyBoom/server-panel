package reconcile

import (
	"strings"
	"testing"

	"ppt/server-panel/services/caddy/config"
	"ppt/server-panel/services/caddy/db"
)

func testCfg() config.Config {
	return config.Config{
		PanelDomain: "cp.propertyweb.co",
		StackPorts: map[string]string{
			"phalcon": "127.0.0.1:8002",
			"laravel": "127.0.0.1:8004",
			"golang":  "127.0.0.1:8005",
		},
	}
}

func names(fs []FileOp) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Name
	}
	return out
}

func TestBuildPlan_RendersActiveAcrossTables(t *testing.T) {
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "tenant.com", ServerStack: "golang", IsActive: true},
		{Table: "platform_hosts", Host: "la-app.propertyboom.co", ServerStack: "laravel", Target: "127.0.0.1:8004", IsActive: true},
		{Table: "platform_redirect_hosts", Host: "old.com", Target: "https://new.com", Code: 301, IsActive: true},
	}}
	p := BuildPlan(testCfg(), snap, nil)

	if len(p.Writes) != 3 {
		t.Fatalf("Writes = %v, want 3", names(p.Writes))
	}
	byName := map[string]string{}
	for _, w := range p.Writes {
		byName[w.Name] = w.Contents
	}
	if got := byName["tenant.com.caddy"]; got != "tenant.com {\n    reverse_proxy 127.0.0.1:8005\n}\n" {
		t.Errorf("tenant render = %q", got)
	}
	if got := byName["old.com.caddy"]; got != "old.com {\n    redir https://new.com 301\n}\n" {
		t.Errorf("redirect render = %q", got)
	}
	if len(p.Removes) != 0 || len(p.Orphans) != 0 {
		t.Errorf("expected no removes/orphans; got removes=%v orphans=%v", p.Removes, p.Orphans)
	}
}

func TestBuildPlan_UnknownStackSkippedNeverGuessed(t *testing.T) {
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "mystery.com", ServerStack: "perl", IsActive: true},
		{Table: "website_hosts", Host: "ok.com", ServerStack: "phalcon", IsActive: true},
	}}
	p := BuildPlan(testCfg(), snap, nil)

	if len(p.Writes) != 1 || p.Writes[0].Name != "ok.com.caddy" {
		t.Fatalf("only the known-stack host should render; got %v", names(p.Writes))
	}
	if len(p.Skips) != 1 || p.Skips[0].Host != "mystery.com" {
		t.Fatalf("unknown-stack row must be skipped+reported; got %+v", p.Skips)
	}
}

func TestBuildPlan_PanelDomainNeverAFile(t *testing.T) {
	// The panel domain is the sole protected host — never rendered/removed/orphaned.
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "platform_hosts", Host: "cp.propertyweb.co", ServerStack: "system", Target: "127.0.0.1:2205", IsActive: true},
	}}
	p := BuildPlan(testCfg(), snap, []string{"cp.propertyweb.co.caddy"})

	if len(p.Writes) != 0 {
		t.Errorf("panel domain must never be rendered to a file; got %v", names(p.Writes))
	}
	if len(p.Removes) != 0 {
		t.Errorf("panel domain file must never be removed; got %v", p.Removes)
	}
	if len(p.Orphans) != 0 {
		t.Errorf("panel file must not be reported as an orphan (it's protected); got %v", p.Orphans)
	}
}

func TestBuildPlan_StackDashboardDomainRendersLikeAPeer(t *testing.T) {
	// app.propertyboom.co is just the phalcon stack's dashboard domain — a peer to
	// go-app/la-app/rust-app — NOT protected, so a platform_hosts row for it renders
	// to a folder file exactly like the others.
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "platform_hosts", Host: "app.propertyboom.co", ServerStack: "phalcon", Target: "127.0.0.1:8002", IsActive: true},
	}}
	p := BuildPlan(testCfg(), snap, nil)

	if len(p.Writes) != 1 || p.Writes[0].Name != "app.propertyboom.co.caddy" {
		t.Fatalf("stack dashboard domain must render like a peer; got %v", names(p.Writes))
	}
}

func TestBuildPlan_RemovesOnlyKnownDisabledWithFile(t *testing.T) {
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "gone.com", ServerStack: "phalcon", IsActive: false},
		{Table: "website_hosts", Host: "deleted.com", ServerStack: "phalcon", IsActive: true, SoftDeleted: true},
		{Table: "website_hosts", Host: "stillhere.com", ServerStack: "phalcon", IsActive: true},
	}}
	folder := []string{"gone.com.caddy", "deleted.com.caddy", "stillhere.com.caddy", "unmanaged.com.caddy"}
	p := BuildPlan(testCfg(), snap, folder)

	wantRemoves := map[string]bool{"gone.com.caddy": true, "deleted.com.caddy": true}
	if len(p.Removes) != 2 {
		t.Fatalf("Removes = %v, want the 2 disabled-with-file", p.Removes)
	}
	for _, r := range p.Removes {
		if !wantRemoves[r] {
			t.Errorf("unexpected remove %q", r)
		}
	}
	if len(p.Orphans) != 1 || p.Orphans[0] != "unmanaged.com.caddy" {
		t.Errorf("Orphans = %v, want [unmanaged.com.caddy]", p.Orphans)
	}
	if len(p.Writes) != 1 || p.Writes[0].Name != "stillhere.com.caddy" {
		t.Errorf("Writes = %v", names(p.Writes))
	}
}

func TestBuildPlanWithKnown_DeletedTenantBecomesRemove(t *testing.T) {
	// A tenant mapping previously desired (knownDesired) and now GONE from the DB
	// (website_hosts hard-delete) — with a healthy non-empty read — is a REMOVE, not
	// an orphan, so the deleted site stops serving. A never-seen file stays orphan.
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "stillhere.com", ServerStack: "phalcon", IsActive: true},
	}}
	folder := []string{"stillhere.com.caddy", "deleted-tenant.com.caddy", "foreign.com.caddy"}
	known := map[string]bool{"deleted-tenant.com": true}
	p := BuildPlanWithKnown(testCfg(), snap, folder, known)

	if len(p.Removes) != 1 || p.Removes[0] != "deleted-tenant.com.caddy" {
		t.Errorf("Removes = %v, want [deleted-tenant.com.caddy]", p.Removes)
	}
	if len(p.Orphans) != 1 || p.Orphans[0] != "foreign.com.caddy" {
		t.Errorf("Orphans = %v, want [foreign.com.caddy] (never seen backed → stays orphan)", p.Orphans)
	}
}

func TestBuildPlanWithKnown_EmptyReadNeverReclassifies(t *testing.T) {
	// Suspicious empty desired set (DB blip): even a previously-desired host stays a
	// conservative orphan — never a mass wipe.
	folder := []string{"deleted-tenant.com.caddy"}
	known := map[string]bool{"deleted-tenant.com": true}
	p := BuildPlanWithKnown(testCfg(), db.Snapshot{}, folder, known)
	if len(p.Removes) != 0 {
		t.Errorf("empty read must reclassify nothing; Removes = %v", p.Removes)
	}
	if len(p.Orphans) != 1 || p.Orphans[0] != "deleted-tenant.com.caddy" {
		t.Errorf("Orphans = %v, want the file kept as an orphan on an empty read", p.Orphans)
	}
}

func TestBuildPlan_DisabledButNoFileIsNotARemove(t *testing.T) {
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "gone.com", ServerStack: "phalcon", IsActive: false},
	}}
	p := BuildPlan(testCfg(), snap, nil)
	if len(p.Removes) != 0 {
		t.Errorf("no file on disk → nothing to remove; got %v", p.Removes)
	}
}

func TestBuildPlan_ActiveRowWinsOverDisabledSameHost(t *testing.T) {
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "dual.com", ServerStack: "phalcon", IsActive: false},
		{Table: "platform_hosts", Host: "dual.com", ServerStack: "phalcon", Target: "127.0.0.1:8002", IsActive: true},
	}}
	p := BuildPlan(testCfg(), snap, []string{"dual.com.caddy"})
	if len(p.Removes) != 0 {
		t.Errorf("active row must win — no remove; got %v", p.Removes)
	}
	if len(p.Writes) != 1 {
		t.Errorf("expected dual.com written; got %v", names(p.Writes))
	}
}

func TestBuildPlan_WildcardFileNeverRemoved(t *testing.T) {
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "*.example.com", ServerStack: "phalcon", IsActive: false},
	}}
	p := BuildPlan(testCfg(), snap, []string{"wildcard_example.com.caddy"})
	if len(p.Removes) != 0 {
		t.Errorf("wildcard files are reserved — never removed; got %v", p.Removes)
	}
	if len(p.Orphans) != 0 {
		t.Errorf("wildcard files are protected, not orphan-reported; got %v", p.Orphans)
	}
}

func TestBuildPlan_DuplicateHostAcrossTablesKeepsFirst(t *testing.T) {
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "dup.com", ServerStack: "phalcon", IsActive: true},
		{Table: "platform_hosts", Host: "dup.com", ServerStack: "phalcon", Target: "127.0.0.1:9999", IsActive: true},
	}}
	p := BuildPlan(testCfg(), snap, nil)
	if len(p.Writes) != 1 {
		t.Fatalf("duplicate host must render once; got %v", names(p.Writes))
	}
	if p.Writes[0].Contents != "dup.com {\n    reverse_proxy 127.0.0.1:8002\n}\n" {
		t.Errorf("first (website_hosts) should win; got %q", p.Writes[0].Contents)
	}
	if len(p.Skips) != 1 {
		t.Errorf("the clashing second row should be reported; got %+v", p.Skips)
	}
}

func TestBuildPlan_EncodeAppliedToProxiesNotRedirects(t *testing.T) {
	cfg := testCfg()
	cfg.Encode = "zstd gzip"
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "tenant.com", ServerStack: "golang", IsActive: true},
		{Table: "platform_hosts", Host: "sys.com", ServerStack: "phalcon", Target: "127.0.0.1:8002", IsActive: true},
		{Table: "platform_redirect_hosts", Host: "old.com", Target: "https://new.com", Code: 301, IsActive: true},
	}}
	p := BuildPlan(cfg, snap, nil)
	byName := map[string]string{}
	for _, w := range p.Writes {
		byName[w.Name] = w.Contents
	}
	if got := byName["tenant.com.caddy"]; got != "tenant.com {\n    encode zstd gzip\n    reverse_proxy 127.0.0.1:8005\n}\n" {
		t.Errorf("tenant encode = %q", got)
	}
	if got := byName["sys.com.caddy"]; got != "sys.com {\n    encode zstd gzip\n    reverse_proxy 127.0.0.1:8002\n}\n" {
		t.Errorf("system encode = %q", got)
	}
	if got := byName["old.com.caddy"]; got != "old.com {\n    redir https://new.com 301\n}\n" {
		t.Errorf("redirect must not get encode; got %q", got)
	}
}

func TestBuildPlan_SecurityHeadersTenantOnly(t *testing.T) {
	cfg := testCfg()
	cfg.SecurityHeaders = &config.SecurityHeaders{Baseline: true}
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "website_hosts", Host: "tenant.com", ServerStack: "golang", IsActive: true},
		{Table: "platform_hosts", Host: "admin.propertyboom.co", ServerStack: "phalcon", Target: "127.0.0.1:8002", IsActive: true},
		{Table: "platform_redirect_hosts", Host: "old.com", Target: "https://new.com", Code: 301, IsActive: true},
	}}
	p := BuildPlan(cfg, snap, nil)
	byName := map[string]string{}
	for _, w := range p.Writes {
		byName[w.Name] = w.Contents
	}
	if !strings.Contains(byName["tenant.com.caddy"], "header {") ||
		!strings.Contains(byName["tenant.com.caddy"], "X-Content-Type-Options nosniff") {
		t.Errorf("tenant should get security headers; got %q", byName["tenant.com.caddy"])
	}
	if strings.Contains(byName["admin.propertyboom.co.caddy"], "header {") {
		t.Errorf("system host must NOT get security headers; got %q", byName["admin.propertyboom.co.caddy"])
	}
	if byName["old.com.caddy"] != "old.com {\n    redir https://new.com 301\n}\n" {
		t.Errorf("redirect must be unchanged; got %q", byName["old.com.caddy"])
	}
}

func TestBuildPlan_PanelDomainProtected(t *testing.T) {
	cfg := testCfg()
	cfg.PanelDomain = "cp.propertyweb.co"
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "platform_hosts", Host: "cp.propertyweb.co", ServerStack: "phalcon", Target: "127.0.0.1:2205", IsActive: true},
	}}
	p := BuildPlan(cfg, snap, []string{"cp.propertyweb.co.caddy"})
	if len(p.Writes) != 0 {
		t.Errorf("panel domain must never render to a file; got %v", names(p.Writes))
	}
	if len(p.Removes) != 0 || len(p.Orphans) != 0 {
		t.Errorf("panel domain file must be protected (no remove/orphan); removes=%v orphans=%v", p.Removes, p.Orphans)
	}
}

func TestBuildPlan_EmptyTargetsSkipped(t *testing.T) {
	snap := db.Snapshot{Rows: []db.Row{
		{Table: "platform_hosts", Host: "sys.com", ServerStack: "phalcon", Target: "", IsActive: true},
		{Table: "platform_redirect_hosts", Host: "red.com", Target: "", Code: 301, IsActive: true},
	}}
	p := BuildPlan(testCfg(), snap, nil)
	if len(p.Writes) != 0 {
		t.Errorf("rows with empty targets must not render; got %v", names(p.Writes))
	}
	if len(p.Skips) != 2 {
		t.Errorf("both empty-target rows should be skipped+reported; got %+v", p.Skips)
	}
}
