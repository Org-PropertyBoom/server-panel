package services

import (
	"context"
	"time"

	caddyconfig "ppt/server-panel/services/caddy/config"
	caddydb "ppt/server-panel/services/caddy/db"
	"ppt/server-panel/services/caddy/reconcile"
)

// VhostEngineService is the Phase-1 (read-only) Caddy vhost engine wiring: it
// reads desired state from a chosen Data Source (by name) and reports DRIFT
// against the vhosts folder. It writes nothing and never reloads Caddy — the live
// reconcile+reload path is a separately-gated Phase 2.
//
// The host-source Data Source name is stored in the settings key
// "vhost_data_source"; when unset, State reports that nothing is configured.
type VhostEngineService struct {
	sources  *DataSourceService
	settings *SettingsService
	cfg      caddyconfig.Config
	engine   *reconcile.Engine
}

// NewVhostEngineService builds the read-only engine over the env-configured
// folder + main Caddyfile (defaults: /home/server/.caddy, /etc/caddy/Caddyfile).
func NewVhostEngineService(sources *DataSourceService, settings *SettingsService) *VhostEngineService {
	cfg := caddyconfig.Load()
	return &VhostEngineService{
		sources:  sources,
		settings: settings,
		cfg:      cfg,
		engine:   reconcile.NewEngine(cfg),
	}
}

// VhostStateResult is the read-only drift view returned to the panel.
type VhostStateResult struct {
	Configured bool                    `json:"configured"`
	Source     string                  `json:"source,omitempty"`
	VhostsDir  string                  `json:"vhostsDir"`
	Message    string                  `json:"message,omitempty"`
	Error      string                  `json:"error,omitempty"`
	DryRun     *reconcile.DryRunResult `json:"dryRun,omitempty"`
}

// State resolves the configured host-source Data Source, reads its desired-state
// snapshot, and computes drift against the vhosts folder. It never mutates
// anything. All failure modes are returned in the result (never a 500) so the UI
// can render a precise banner.
func (v *VhostEngineService) State(ctx context.Context) VhostStateResult {
	out := VhostStateResult{VhostsDir: v.cfg.VhostsDir}

	name := v.settings.Get("vhost_data_source", "")
	if name == "" {
		out.Message = "No host-source data source selected. Choose one under Settings → Data Sources."
		return out
	}
	out.Configured = true
	out.Source = name

	ds, ok := v.sources.ResolveByName(name)
	if !ok {
		out.Error = "Selected data source \"" + name + "\" no longer exists."
		return out
	}

	adapter, ok := adapterFor(ds.Engine)
	if !ok {
		out.Error = "Unsupported engine for data source \"" + name + "\"."
		return out
	}

	conn, err := caddydb.Open(ctx, adapter.BuildDSN(ds))
	if err != nil {
		out.Error = friendlyDBError(err)
		return out
	}
	defer conn.Close()

	snap, err := conn.ReadSnapshot(ctx)
	if err != nil {
		out.Error = friendlyDBError(err)
		return out
	}
	snap.ReadAt = time.Now()

	dry, err := v.engine.DryRun(snap)
	if err != nil {
		out.Error = "vhosts folder: " + err.Error()
		return out
	}
	out.DryRun = &dry
	return out
}
