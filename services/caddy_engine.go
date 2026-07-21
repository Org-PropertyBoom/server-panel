package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"ppt/server-panel/services/caddy/caddyctl"
	caddyconfig "ppt/server-panel/services/caddy/config"
	caddydb "ppt/server-panel/services/caddy/db"
	"ppt/server-panel/services/caddy/reconcile"
)

// VhostEngineService wires the Caddy vhost engine into server-panel. It reads
// desired state from a chosen Data Source (by name), computes drift (read-only),
// and — behind the CADDY_LIVE_RELOAD gate — applies changes by rendering files and
// reloading Caddy through the safe adapt-as-root → POST /load path.
//
// It is a SINGLETON (constructed once in main.go): the engine's first-pass removal
// suppression is per-process state that must persist across requests.
//
// The host-source Data Source name is the settings key "vhost_data_source".
type VhostEngineService struct {
	sources  *DataSourceService
	settings *SettingsService
	cfg      caddyconfig.Config
	engine   *reconcile.Engine
}

// NewVhostEngineService builds the engine over the env-configured folder + main
// Caddyfile, with the shell adapter (`caddy adapt` as root) + admin-API reloader.
func NewVhostEngineService(sources *DataSourceService, settings *SettingsService) *VhostEngineService {
	cfg := caddyconfig.Load()
	engine := reconcile.NewEngine(cfg, caddyctl.Adapter{}, caddyctl.NewClient(cfg.CaddyAdminURL))
	return &VhostEngineService{sources: sources, settings: settings, cfg: cfg, engine: engine}
}

// LiveReloadEnabled reports whether the live reconcile+reload path is switched on.
// Default OFF — the code ships inert; an operator enables it with CADDY_LIVE_RELOAD=1
// once the mounts/infra are present (a deliberate, per-host activation).
func (v *VhostEngineService) LiveReloadEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CADDY_LIVE_RELOAD"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// SystemHostForm / RedirectForm are the management-UI create/update payloads.
type SystemHostForm struct {
	ID          int64  `json:"id"`
	Host        string `json:"host"`
	ServerStack string `json:"serverStack"`
	Target      string `json:"target"`
	IsActive    bool   `json:"isActive"`
}

type RedirectForm struct {
	ID       int64  `json:"id"`
	Host     string `json:"host"`
	Target   string `json:"target"`
	Code     int    `json:"code"`
	IsActive bool   `json:"isActive"`
}

// ManageRow is one editable desired-state row (platform_hosts or
// platform_redirect_hosts) surfaced to the management UI, carrying the DB primary
// key so edit/delete can target it. website_hosts are NOT included — they are
// stack-owned and read-only here.
type ManageRow struct {
	ID          int64  `json:"id"`
	Host        string `json:"host"`
	ServerStack string `json:"serverStack,omitempty"`
	Target      string `json:"target"`
	Code        int    `json:"code,omitempty"`
	IsActive    bool   `json:"isActive"`
	SoftDeleted bool   `json:"softDeleted"`
}

// ManageSets is the editable slice of desired state: the panel-owned system hosts
// and redirects, plus the known stacks the UI offers for a system host.
type ManageSets struct {
	SystemHosts []ManageRow `json:"systemHosts"`
	Redirects   []ManageRow `json:"redirects"`
	Stacks      []string    `json:"stacks"`
}

// VhostStateResult is the read-only drift view returned to the panel.
type VhostStateResult struct {
	Configured bool                    `json:"configured"`
	Source     string                  `json:"source,omitempty"`
	VhostsDir  string                  `json:"vhostsDir"`
	LiveReload bool                    `json:"liveReload"`
	Message    string                  `json:"message,omitempty"`
	Error      string                  `json:"error,omitempty"`
	DryRun     *reconcile.DryRunResult `json:"dryRun,omitempty"`
	Manage     *ManageSets             `json:"manage,omitempty"`
}

// State resolves the configured host-source Data Source, reads its desired-state
// snapshot, and computes drift against the vhosts folder. It never mutates
// anything. Failure modes are returned in the result (never a 500).
func (v *VhostEngineService) State(ctx context.Context) VhostStateResult {
	out := VhostStateResult{VhostsDir: v.cfg.VhostsDir, LiveReload: v.LiveReloadEnabled()}

	name := v.settings.Get("vhost_data_source", "")
	if name == "" {
		out.Message = "No host-source data source selected. Choose one under Settings → Data Sources."
		return out
	}
	out.Configured = true
	out.Source = name

	conn, err := v.openDB(ctx)
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
	out.Manage = v.manageSets(snap)
	return out
}

// manageSets projects the snapshot's panel-owned rows (platform_hosts +
// platform_redirect_hosts) into the editable UI shape, carrying primary keys.
// website_hosts are intentionally excluded — stack-owned, read-only here.
func (v *VhostEngineService) manageSets(snap caddydb.Snapshot) *ManageSets {
	m := &ManageSets{SystemHosts: []ManageRow{}, Redirects: []ManageRow{}, Stacks: v.cfg.Stacks()}
	for _, r := range snap.Rows {
		if r.SoftDeleted {
			continue
		}
		switch r.Table {
		case "platform_hosts":
			m.SystemHosts = append(m.SystemHosts, ManageRow{
				ID: r.ID, Host: r.Host, ServerStack: r.ServerStack, Target: r.Target,
				IsActive: r.IsActive, SoftDeleted: r.SoftDeleted,
			})
		case "platform_redirect_hosts":
			m.Redirects = append(m.Redirects, ManageRow{
				ID: r.ID, Host: r.Host, Target: r.Target, Code: r.Code,
				IsActive: r.IsActive, SoftDeleted: r.SoftDeleted,
			})
		}
	}
	return m
}

// Reconcile applies desired state: render → validate (adapt) → backup → reload.
// GATED: refuses unless CADDY_LIVE_RELOAD is on.
func (v *VhostEngineService) Reconcile(ctx context.Context) (reconcile.Result, error) {
	if !v.LiveReloadEnabled() {
		return reconcile.Result{Error: liveGateMsg}, errLiveGate
	}
	conn, err := v.openDB(ctx)
	if err != nil {
		return reconcile.Result{Error: friendlyDBError(err)}, err
	}
	defer conn.Close()
	snap, err := conn.ReadSnapshot(ctx)
	if err != nil {
		return reconcile.Result{Error: friendlyDBError(err)}, err
	}
	snap.ReadAt = time.Now()
	return v.engine.Reconcile(ctx, snap)
}

// ReloadOnly re-validates and reloads the current folder. GATED.
func (v *VhostEngineService) ReloadOnly(ctx context.Context) (reconcile.Result, error) {
	if !v.LiveReloadEnabled() {
		return reconcile.Result{Error: liveGateMsg}, errLiveGate
	}
	return v.engine.ReloadOnly(ctx)
}

// SaveSystemHost creates (ID==0) or updates a platform_hosts row. This is a DB
// write only — the change becomes live on the next Reconcile.
func (v *VhostEngineService) SaveSystemHost(ctx context.Context, f SystemHostForm) error {
	in, err := caddydb.ValidateSystemHost(caddydb.SystemHostInput{
		Host: f.Host, ServerStack: f.ServerStack, Target: f.Target, IsActive: f.IsActive,
	}, v.guard())
	if err != nil {
		return err
	}
	conn, err := v.openDB(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if f.ID == 0 {
		_, err = conn.CreateSystemHost(ctx, in)
		return err
	}
	return conn.UpdateSystemHost(ctx, f.ID, in)
}

// SaveRedirect creates (ID==0) or updates a platform_redirect_hosts row.
func (v *VhostEngineService) SaveRedirect(ctx context.Context, f RedirectForm) error {
	in, err := caddydb.ValidateRedirect(caddydb.RedirectInput{
		Host: f.Host, Target: f.Target, Code: f.Code, IsActive: f.IsActive,
	}, v.guard())
	if err != nil {
		return err
	}
	conn, err := v.openDB(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if f.ID == 0 {
		_, err = conn.CreateRedirect(ctx, in)
		return err
	}
	return conn.UpdateRedirect(ctx, f.ID, in)
}

// DeleteSystemHost / DeleteRedirect soft-delete a row (removal applies on the next
// non-first Reconcile).
func (v *VhostEngineService) DeleteSystemHost(ctx context.Context, id int64) error {
	conn, err := v.openDB(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.DeleteSystemHost(ctx, id)
}

func (v *VhostEngineService) DeleteRedirect(ctx context.Context, id int64) error {
	conn, err := v.openDB(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.DeleteRedirect(ctx, id)
}

// PruneOrphan removes one orphan `<host>.caddy` file (refusing protected/wildcard),
// then reconciles to validate + reload. GATED.
func (v *VhostEngineService) PruneOrphan(ctx context.Context, name string) (reconcile.Result, error) {
	if !v.LiveReloadEnabled() {
		return reconcile.Result{Error: liveGateMsg}, errLiveGate
	}
	if _, err := v.engine.RemoveFile(name); err != nil {
		return reconcile.Result{Error: err.Error()}, err
	}
	return v.Reconcile(ctx)
}

func (v *VhostEngineService) openDB(ctx context.Context) (*caddydb.DB, error) {
	name := v.settings.Get("vhost_data_source", "")
	if name == "" {
		return nil, errors.New("no host-source data source selected")
	}
	ds, ok := v.sources.ResolveByName(name)
	if !ok {
		return nil, fmt.Errorf("data source %q no longer exists", name)
	}
	adapter, ok := adapterFor(ds.Engine)
	if !ok {
		return nil, fmt.Errorf("unsupported engine for data source %q", name)
	}
	return caddydb.Open(ctx, adapter.BuildDSN(ds))
}

func (v *VhostEngineService) guard() caddydb.Guard {
	return caddydb.Guard{
		IsProtected: func(h string) bool {
			for _, p := range v.cfg.ProtectedHosts() {
				if strings.EqualFold(p, h) {
					return true
				}
			}
			return false
		},
		StackKnown: func(s string) bool {
			_, ok := v.cfg.UpstreamFor(s)
			return ok
		},
	}
}

var errLiveGate = errors.New("live reconcile is not enabled")

const liveGateMsg = "Live reconcile is not enabled on this server. Set CADDY_LIVE_RELOAD=1 in the root env and restart to activate the write + validated-reload path."
