package services

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	caddyconfig "ppt/server-panel/services/caddy/config"
)

// newCertAskEngine builds a VhostEngineService with panel-local settings and an
// EMPTY data source list — no reachable shared DB. That is deliberate: every
// assertion below must hold without MySQL, which also proves the decision it tests
// is made before CertAllowed ever opens the shared DB. The data-source path is
// redirected to the temp dir so the test never reads the real config.
func newCertAskEngine(t *testing.T) *VhostEngineService {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(settingsDBEnv, filepath.Join(dir, ".ppt-server-panel", "data", "db.sqlite"))
	t.Setenv(datasourcesPathVar, filepath.Join(dir, "datasources.json"))
	settings, err := NewSettingsService()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { settings.db.Close() })
	return &VhostEngineService{
		sources:      NewDataSourceService(),
		settings:     settings,
		cfg:          caddyconfig.Config{},
		certAskCache: map[string]time.Time{},
	}
}

// A deny must never lose to a stale allow. The allow-cache used to be consulted
// before the suppression check, so a host cached as allowed kept answering 200
// after the operator disabled it, until the 12h TTL expired.
func TestCertAllowedExplicitDisableBeatsCachedAllow(t *testing.T) {
	v := newCertAskEngine(t)
	const host = "dead.example.com"

	v.certAskStore(host)
	if !v.certAskCached(host) {
		t.Fatal("precondition: host should be cached as allowed")
	}
	if err := v.settings.SetHostSuppressed(host, true); err != nil {
		t.Fatal(err)
	}

	allowed, err := v.CertAllowed(context.Background(), host)
	if err != nil {
		t.Fatalf("CertAllowed returned an error: %v", err)
	}
	if allowed {
		t.Fatal("a suppressed host was authorized from the allow-cache")
	}
}

// The reorder must not cost the availability property the cache exists for: a
// known-good host still answers without touching the shared DB. There is no data
// source configured here, so if this reached openDB it would error instead.
func TestCertAllowedCachedHostAnswersWithoutSharedDB(t *testing.T) {
	v := newCertAskEngine(t)
	const host = "live.example.com"

	v.certAskStore(host)

	allowed, err := v.CertAllowed(context.Background(), host)
	if err != nil {
		t.Fatalf("cached host should not have consulted the shared DB, got error: %v", err)
	}
	if !allowed {
		t.Fatal("cached host was not authorized")
	}
}

// An unknown host stays fail-closed — the abuse guard. Without a data source the
// DB path errors, which CertAllowed must surface as "not allowed".
func TestCertAllowedUnknownHostIsNotAuthorized(t *testing.T) {
	v := newCertAskEngine(t)

	allowed, _ := v.CertAllowed(context.Background(), "never-seen.example.com")
	if allowed {
		t.Fatal("an unknown host was authorized")
	}
}

// Eviction has to clear the PERSISTED allowlist too, not just memory — otherwise a
// panel restart reloads the entry and re-authorizes a removed host.
func TestCertAskEvictClearsMemoryAndSurvivesRestart(t *testing.T) {
	v := newCertAskEngine(t)
	const host = "removed.example.com"

	v.certAskStore(host)
	persisted, err := v.settings.TLSAskAllowlist()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := persisted[host]; !ok {
		t.Fatal("precondition: host should have been persisted to the allowlist")
	}

	v.certAskEvict(host)

	if v.certAskCached(host) {
		t.Fatal("host still cached in memory after evict")
	}
	persisted, err = v.settings.TLSAskAllowlist()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := persisted[host]; ok {
		t.Fatal("host still in the persisted allowlist after evict — a restart would re-authorize it")
	}

	// Simulate a panel restart: a fresh engine over the same settings must not
	// resurrect the entry via certAskLoad.
	restarted := &VhostEngineService{
		sources:      v.sources,
		settings:     v.settings,
		cfg:          v.cfg,
		certAskCache: map[string]time.Time{},
	}
	if restarted.certAskCached(host) {
		t.Fatal("evicted host came back after a restart")
	}
}

// certAskEvictHost is called with the result of an id->host lookup that may have
// failed, so it must ignore an empty host rather than acting on "".
func TestCertAskEvictHostIgnoresEmptyAndNormalizes(t *testing.T) {
	v := newCertAskEngine(t)
	const host = "mixed.example.com"

	v.certAskStore(host)
	v.certAskEvictHost("   ")
	if !v.certAskCached(host) {
		t.Fatal("an empty host evicted an unrelated entry")
	}

	// The write paths pass the host as stored in the DB, which may differ in case
	// from the normalized allowlist key.
	v.certAskEvictHost("  Mixed.Example.COM  ")
	if v.certAskCached(host) {
		t.Fatal("evict did not normalize the host key")
	}
}
