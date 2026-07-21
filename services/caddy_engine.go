package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"ppt/server-panel/services/caddy/caddyctl"
	caddyconfig "ppt/server-panel/services/caddy/config"
	caddydb "ppt/server-panel/services/caddy/db"
	caddyhealth "ppt/server-panel/services/caddy/health"
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
	sources    *DataSourceService
	settings   *SettingsService
	cfg        caddyconfig.Config
	engine     *reconcile.Engine
	health     *HealthProbeService // alert-only reachability probe (read-only), attached post-construction
	containers *ContainerService   // for the system-host upstream picker (running containers + published ports)
}

// AttachHealth wires the reachability probe so State can surface it. Set once at
// boot; nil-safe if never attached.
func (v *VhostEngineService) AttachHealth(h *HealthProbeService) { v.health = h }

// TenantHosts returns the active website_hosts hostnames — the set the health
// probe checks for reachability. Read-only.
func (v *VhostEngineService) TenantHosts(ctx context.Context) ([]string, error) {
	conn, err := v.openDB(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	snap, err := conn.ReadSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range snap.Rows {
		if r.Table == "website_hosts" && r.Desired() {
			out = append(out, r.Host)
		}
	}
	return out, nil
}

// NewVhostEngineService builds the engine over the env-configured folder + main
// Caddyfile, with the shell adapter (`caddy adapt` as root) + admin-API reloader.
func NewVhostEngineService(sources *DataSourceService, settings *SettingsService) *VhostEngineService {
	cfg := caddyconfig.Load()
	engine := reconcile.NewEngine(cfg, caddyctl.Adapter{}, caddyctl.NewClient(cfg.CaddyAdminURL))
	return &VhostEngineService{sources: sources, settings: settings, cfg: cfg, engine: engine, containers: NewContainerService()}
}

// Upstream is one reverse-proxy target a system host can point at — synced from a
// running container's published host port. The set is broader than the code
// stacks: any container (nocodb, phpmyadmin, minio, …) is a valid system upstream.
type Upstream struct {
	Name   string `json:"name"`   // container name (display label)
	Target string `json:"target"` // 127.0.0.1:<published host port>
}

// containerUpstreams lists the running containers' published host ports as
// selectable upstreams (deduped by target, sorted). Server-panel is the source of
// truth for what's runnable here — so the system-host picker stays in sync with the
// host instead of a hardcoded stack list.
func (v *VhostEngineService) containerUpstreams() []Upstream {
	var out []Upstream
	seen := map[string]bool{}
	for _, c := range v.containers.ListAll() {
		if !containerRunning(c) {
			continue
		}
		for _, port := range publishedHostPorts(c.Ports) {
			target := "127.0.0.1:" + port
			if seen[target] {
				continue
			}
			seen[target] = true
			out = append(out, Upstream{Name: c.Name, Target: target})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Target < out[j].Target
	})
	return out
}

func containerRunning(c Container) bool {
	return strings.EqualFold(strings.TrimSpace(c.State), "running") ||
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.Status)), "up")
}

// publishedHostPorts extracts the distinct host ports from docker/podman port
// strings like "0.0.0.0:9001->8080/tcp" or "[::]:9001->8080/tcp" (entries without
// a "->" are container-internal and skipped).
func publishedHostPorts(ports []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range ports {
		arrow := strings.Index(p, "->")
		if arrow < 0 {
			continue
		}
		left := p[:arrow]
		colon := strings.LastIndex(left, ":")
		if colon < 0 {
			continue
		}
		port := strings.TrimSpace(left[colon+1:])
		if port == "" || seen[port] || !isAllDigits(port) {
			continue
		}
		seen[port] = true
		out = append(out, port)
	}
	return out
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

const liveReloadSettingKey = "vhost_live_reload"

// LiveReloadEnabled reports whether the live reconcile+reload path is switched on.
// The runtime source of truth is the persisted setting (toggled from the UI, takes
// effect immediately, no restart). Until it has EVER been set, the value is seeded
// from the CADDY_LIVE_RELOAD env var — so an install already env-armed stays armed
// when this ships. Default OFF.
func (v *VhostEngineService) LiveReloadEnabled() bool {
	switch v.settings.Get(liveReloadSettingKey, "") {
	case "true":
		return true
	case "false":
		return false
	}
	return envLiveReloadArmed()
}

// SetLiveReload persists the runtime gate. After the first toggle the setting is
// authoritative; the env var only seeds the never-set case. Disarming is always
// safe (re-inerts the engine; on-disk files stay).
func (v *VhostEngineService) SetLiveReload(enabled bool) error {
	return v.settings.Set(liveReloadSettingKey, strconv.FormatBool(enabled))
}

func envLiveReloadArmed() bool {
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
	Upstreams   []Upstream  `json:"upstreams"` // running containers a system host can proxy to
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
	// Health is the alert-only reachability status per host (DNS + TLS), orthogonal
	// to reconcile drift. Empty/absent when the probe is disabled or hasn't run.
	Health   map[string]caddyhealth.Status `json:"health,omitempty"`
	HealthOn bool                          `json:"healthOn"`
}

// State resolves the configured host-source Data Source, reads its desired-state
// snapshot, and computes drift against the vhosts folder. It never mutates
// anything. Failure modes are returned in the result (never a 500).
func (v *VhostEngineService) State(ctx context.Context) VhostStateResult {
	out := VhostStateResult{VhostsDir: v.cfg.VhostsDir, LiveReload: v.LiveReloadEnabled(), HealthOn: v.health.Enabled()}
	out.Health = v.health.Snapshot()

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
	m := &ManageSets{SystemHosts: []ManageRow{}, Redirects: []ManageRow{}, Stacks: v.cfg.Stacks(), Upstreams: v.containerUpstreams()}
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

// renderedStatusVersion is the stable schema version of the stack-facing feed.
// Bump on any breaking change to RenderedStatusResult so all 3 stacks can gate.
const renderedStatusVersion = "1"

// RenderedHostStatus is one host's DB-vs-file standing for stack consumption.
type RenderedHostStatus struct {
	Host        string `json:"host"`
	Kind        string `json:"kind,omitempty"`        // tenant | system | redirect | orphan
	Status      string `json:"status"`                // in_sync | will_write | will_remove | orphan
	HasFile     bool   `json:"hasFile"`               // a matching <host>.caddy exists on disk
	WebsiteID   int64  `json:"websiteId,omitempty"`   // tenant only: website_hosts.website_id
	WebsiteName string `json:"websiteName,omitempty"` // tenant only: websites.name
}

// RenderedStatusResult is the read-only feed the stack apps consume to badge their
// own DB-vhost tables — "does this DB vhost have a matching rendered vhost file?"
// It is sourced entirely from server-panel (the folder it owns + the shared DB it
// reads), NEVER from Caddy. RenderedHosts is always present (a folder read needs no
// DB); Hosts carries the richer per-host status when a data source is selected.
type RenderedStatusResult struct {
	Version       string               `json:"version"` // stable schema version for the 3 stacks
	VhostsDir     string               `json:"vhostsDir"`
	Source        string               `json:"source,omitempty"`
	RenderedHosts []string             `json:"renderedHosts"`
	Hosts         []RenderedHostStatus `json:"hosts"`
	Error         string               `json:"error,omitempty"`
}

// RenderedStatus reports the rendered vhosts on disk and, when a host-source is
// configured, each host's DB-vs-file status. Read-only; failure modes are returned
// in the result (the rendered list still comes back even if the DB join fails).
func (v *VhostEngineService) RenderedStatus(ctx context.Context) RenderedStatusResult {
	out := RenderedStatusResult{Version: renderedStatusVersion, VhostsDir: v.cfg.VhostsDir, RenderedHosts: []string{}, Hosts: []RenderedHostStatus{}}

	phys, err := v.engine.RenderedHosts()
	if err != nil {
		out.Error = "vhosts folder: " + err.Error()
		return out
	}
	out.RenderedHosts = phys

	name := v.settings.Get("vhost_data_source", "")
	if name == "" {
		return out // rendered list only — no data source to compute per-host status
	}
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
	physSet := make(map[string]bool, len(phys))
	for _, h := range phys {
		physSet[h] = true
	}
	// Website mapping (id + name) per tenant host, keyed lowercase for a safe join.
	type mapping struct {
		id   int64
		name string
	}
	websiteBy := make(map[string]mapping)
	for _, r := range snap.Rows {
		if r.Table == "website_hosts" {
			websiteBy[strings.ToLower(strings.TrimSpace(r.Host))] = mapping{id: r.WebsiteID, name: r.WebsiteName}
		}
	}
	for _, h := range dry.Hosts {
		row := RenderedHostStatus{Host: h.Hostname, Kind: h.Kind, Status: h.Status, HasFile: physSet[h.Hostname]}
		if m, ok := websiteBy[strings.ToLower(strings.TrimSpace(h.Hostname))]; ok {
			row.WebsiteID, row.WebsiteName = m.id, m.name
		}
		out.Hosts = append(out.Hosts, row)
	}
	return out
}

// targetRoutes is the set of host routes pointing at one upstream target.
type targetRoutes struct {
	appHosts    []string
	tenantCount int
	tenantStack string
}

// routesByTarget indexes desired host routes by their upstream target
// ("127.0.0.1:port"): App routes (platform_hosts.target) contribute hostnames;
// tenant routes (website_hosts, via server_stack→port) contribute a count. Read-only.
func (v *VhostEngineService) routesByTarget(ctx context.Context) (map[string]targetRoutes, error) {
	if v.settings.Get("vhost_data_source", "") == "" {
		return nil, nil
	}
	conn, err := v.openDB(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	snap, err := conn.ReadSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]targetRoutes{}
	for _, r := range snap.Rows {
		if !r.Desired() {
			continue
		}
		switch r.Table {
		case "website_hosts":
			up, ok := v.cfg.UpstreamFor(r.ServerStack)
			if !ok {
				continue
			}
			tr := out[up]
			tr.tenantCount++
			tr.tenantStack = strings.ToLower(strings.TrimSpace(r.ServerStack))
			out[up] = tr
		case "platform_hosts":
			t := strings.TrimSpace(r.Target)
			if t == "" {
				continue
			}
			tr := out[t]
			tr.appHosts = append(tr.appHosts, strings.ToLower(strings.TrimSpace(r.Host)))
			out[t] = tr
		}
	}
	for k, tr := range out {
		sort.Strings(tr.appHosts)
		out[k] = tr
	}
	return out, nil
}

// AnnotateContainers fills each container's reverse route view — the hostnames that
// route to it — by matching its published 127.0.0.1:PORT against the host routes.
// Read-only; on any error (no source, DB down) the containers are returned as-is.
func (v *VhostEngineService) AnnotateContainers(ctx context.Context, containers []Container) []Container {
	routes, err := v.routesByTarget(ctx)
	if err != nil || routes == nil {
		return containers
	}
	for i := range containers {
		seen := map[string]bool{}
		var appHosts []string
		tenantCount, tenantStack := 0, ""
		for _, port := range publishedHostPorts(containers[i].Ports) {
			tr, ok := routes["127.0.0.1:"+port]
			if !ok {
				continue
			}
			for _, h := range tr.appHosts {
				if !seen[h] {
					seen[h] = true
					appHosts = append(appHosts, h)
				}
			}
			tenantCount += tr.tenantCount
			if tr.tenantStack != "" {
				tenantStack = tr.tenantStack
			}
		}
		sort.Strings(appHosts)
		containers[i].RouteHosts = appHosts
		containers[i].RouteTenantCount = tenantCount
		containers[i].RouteTenantStack = tenantStack
	}
	return containers
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

// PruneOrphan removes one orphan `<host>.caddy` file, then reconciles. GATED.
func (v *VhostEngineService) PruneOrphan(ctx context.Context, name string) (reconcile.Result, error) {
	return v.PruneOrphans(ctx, []string{name})
}

// PruneOrphans removes the given orphan `<host>.caddy` files (each refusing
// protected/wildcard), then reconciles ONCE to validate + reload. Every removed
// file is recorded as an intentional removal so the drop-guard allows it. GATED.
func (v *VhostEngineService) PruneOrphans(ctx context.Context, names []string) (reconcile.Result, error) {
	if !v.LiveReloadEnabled() {
		return reconcile.Result{Error: liveGateMsg}, errLiveGate
	}
	if len(names) == 0 {
		return reconcile.Result{Error: "no orphan files given"}, errors.New("no orphan files given")
	}
	var refused []string
	for _, name := range names {
		if _, err := v.engine.RemoveFile(name); err != nil {
			refused = append(refused, name+" ("+err.Error()+")")
		}
	}
	res, err := v.Reconcile(ctx)
	if len(refused) > 0 && res.Error == "" {
		res.Error = fmt.Sprintf("skipped %d file(s): %s", len(refused), strings.Join(refused, "; "))
	}
	return res, err
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
