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
	ID          int64             `json:"id"`
	Host        string            `json:"host"`
	ServerStack string            `json:"serverStack"`
	Target      string            `json:"target"`
	IsActive    bool              `json:"isActive"`
	Headers     map[string]string `json:"headers,omitempty"` // panel-local response headers (System hosts only)
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
	ID          int64             `json:"id"`
	Host        string            `json:"host"`
	ServerStack string            `json:"serverStack,omitempty"`
	Target      string            `json:"target"`
	Code        int               `json:"code,omitempty"`
	IsActive    bool              `json:"isActive"`
	SoftDeleted bool              `json:"softDeleted"`
	Headers     map[string]string `json:"headers,omitempty"` // system hosts only: panel-local response headers
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
	// Protected are the pinned domains shown read-only atop the System list. Derived
	// from the ACTUAL main Caddyfile (adapt − folder routes) = ground truth, with a
	// drift flag vs config.ProtectedHosts() (what the reload actually guards).
	Protected        []PinnedRow `json:"protected"`
	ProtectedWarning string      `json:"protectedWarning,omitempty"` // set when the Caddyfile couldn't be adapted (showing config declaration only)
}

// PinnedRow is one pinned/protected domain for the System list.
type PinnedRow struct {
	Host      string   `json:"host"`
	Role      string   `json:"role,omitempty"`      // Panel | Dashboard | Protected (config role)
	Upstreams []string `json:"upstreams,omitempty"` // reverse_proxy dials from the adapted Caddyfile
	Guarded   bool     `json:"guarded"`             // in config.ProtectedHosts() (asserted on every reload)
	Pinned    bool     `json:"pinned"`              // actually a static block in the Caddyfile
	// Drift: "" = in sync; "missing" = guarded but NOT actually pinned (CRITICAL);
	// "unmanaged" = actually pinned but NOT guarded.
	Drift string `json:"drift,omitempty"`
}

// State resolves the configured host-source Data Source, reads its desired-state
// snapshot, and computes drift against the vhosts folder. It never mutates
// anything. Failure modes are returned in the result (never a 500).
func (v *VhostEngineService) State(ctx context.Context) VhostStateResult {
	out := VhostStateResult{VhostsDir: v.cfg.VhostsDir, LiveReload: v.LiveReloadEnabled(), HealthOn: v.health.Enabled()}
	out.Health = v.health.Snapshot()
	out.Protected, out.ProtectedWarning = v.protectedRows()

	ds, ok := v.sources.ActiveSource()
	if !ok {
		out.Message = "No active data source. Add one under Settings → Data Sources."
		return out
	}
	out.Configured = true
	out.Source = ds.Name

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
	snap.ResponseHeaders, _ = v.settings.AllVhostHeaders() // so dry-run render matches what reconcile will write

	dry, err := v.engine.DryRun(snap)
	if err != nil {
		out.Error = "vhosts folder: " + err.Error()
		return out
	}
	out.DryRun = &dry
	out.Manage = v.manageSets(snap)
	return out
}

// protectedRows builds the pinned display from the ACTUAL Caddyfile (ground truth)
// joined with config.ProtectedHosts() (what the reload guards), flagging drift. On
// an adapt failure it falls back to the config declaration + a warning — never
// nothing. config.ProtectedHosts() itself remains the untouched reload invariant.
func (v *VhostEngineService) protectedRows() ([]PinnedRow, string) {
	guardedRole := map[string]string{}
	for _, h := range v.cfg.ProtectedHosts() {
		role := "Protected"
		if strings.EqualFold(h, v.cfg.PanelDomain) {
			role = "Panel"
		}
		guardedRole[strings.ToLower(strings.TrimSpace(h))] = role
	}

	pinned, err := v.engine.PinnedFromCaddyfile()
	if err != nil {
		out := make([]PinnedRow, 0, len(guardedRole))
		for h, role := range guardedRole {
			out = append(out, PinnedRow{Host: h, Role: role, Guarded: true})
		}
		sortPinned(out)
		return out, "Could not adapt the main Caddyfile to verify the pinned set — showing the config declaration only: " + err.Error()
	}

	upstreams := map[string][]string{}
	for _, p := range pinned {
		upstreams[strings.ToLower(strings.TrimSpace(p.Host))] = p.Upstreams
	}

	seen := map[string]bool{}
	var out []PinnedRow
	add := func(h string) {
		if seen[h] {
			return
		}
		seen[h] = true
		role, isGuarded := guardedRole[h]
		ups, isPinned := upstreams[h]
		row := PinnedRow{Host: h, Role: role, Upstreams: ups, Guarded: isGuarded, Pinned: isPinned}
		switch {
		case isGuarded && !isPinned:
			row.Drift = "missing" // guarded but not actually a static block — CRITICAL
		case isPinned && !isGuarded:
			row.Drift = "unmanaged" // a static block we don't guard
		}
		out = append(out, row)
	}
	for h := range guardedRole {
		add(h)
	}
	for h := range upstreams {
		add(h)
	}
	sortPinned(out)
	return out, ""
}

func sortPinned(rows []PinnedRow) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].Host < rows[j].Host })
}

// manageSets projects the snapshot's panel-owned rows (platform_hosts +
// platform_redirect_hosts) into the editable UI shape, carrying primary keys.
// website_hosts are intentionally excluded — stack-owned, read-only here.
func (v *VhostEngineService) manageSets(snap caddydb.Snapshot) *ManageSets {
	m := &ManageSets{SystemHosts: []ManageRow{}, Redirects: []ManageRow{}, Stacks: v.cfg.Stacks(), Upstreams: v.containerUpstreams()}
	headers, _ := v.settings.AllVhostHeaders() // panel-local; nil on error → no headers shown
	for _, r := range snap.Rows {
		if r.SoftDeleted {
			continue
		}
		switch r.Table {
		case "platform_hosts":
			m.SystemHosts = append(m.SystemHosts, ManageRow{
				ID: r.ID, Host: r.Host, ServerStack: r.ServerStack, Target: r.Target,
				IsActive: r.IsActive, SoftDeleted: r.SoftDeleted,
				Headers: headers[strings.ToLower(strings.TrimSpace(r.Host))],
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
	if _, ok := v.sources.ActiveSource(); !ok {
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

// RedirectTarget is a suggested redirect destination — an active tenant website
// domain (with its website name), for the redirect-target combobox.
type RedirectTarget struct {
	Domain    string `json:"domain"`
	Website   string `json:"website,omitempty"`
	WebsiteID int64  `json:"websiteId,omitempty"`
}

// RedirectTargets lists the active tenant domains (website_hosts) as redirect
// destination suggestions, joined to the website name. Read-only; empty when no
// active source. Sorted by domain.
func (v *VhostEngineService) RedirectTargets(ctx context.Context) ([]RedirectTarget, error) {
	if _, ok := v.sources.ActiveSource(); !ok {
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
	var out []RedirectTarget
	for _, r := range snap.Rows {
		if r.Table == "website_hosts" && r.Desired() {
			out = append(out, RedirectTarget{Domain: r.Host, Website: r.WebsiteName, WebsiteID: r.WebsiteID})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out, nil
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
	snap.ResponseHeaders, _ = v.settings.AllVhostHeaders() // panel-local; rendered into system host blocks
	return v.engine.Reconcile(ctx, snap)
}

// ReloadOnly re-validates and reloads the current folder. GATED.
func (v *VhostEngineService) ReloadOnly(ctx context.Context) (reconcile.Result, error) {
	if !v.LiveReloadEnabled() {
		return reconcile.Result{Error: liveGateMsg}, errLiveGate
	}
	return v.engine.ReloadOnly(ctx)
}

// RemovePinnedBlock removes a "Pinned · unmanaged" static block from the main
// Caddyfile (validated + diff-asserted + reloaded). server-panel's only write to
// the operator-owned Caddyfile. GATED by live-reconcile like every write path.
func (v *VhostEngineService) RemovePinnedBlock(ctx context.Context, host string) (reconcile.Result, error) {
	if !v.LiveReloadEnabled() {
		return reconcile.Result{Error: liveGateMsg}, errLiveGate
	}
	return v.engine.RemoveStaticBlock(ctx, host)
}

// PinRoute converts an Active platform_hosts row into a hand-written static
// Caddyfile block (the "unmanaged pinned" representation). Engine-first: the
// static block is added + folder file removed + validated + reloaded BEFORE the DB
// row is dropped, so no window ever serves the host from neither source. If the DB
// delete fails afterwards the host still serves from the new block — surfaced as a
// warning to clean up (an active row would otherwise duplicate the block on a
// reconcile, which adapt refuses fail-safe). GATED.
func (v *VhostEngineService) PinRoute(ctx context.Context, id int64) (reconcile.Result, error) {
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
	var host, target string
	found := false
	for _, r := range snap.Rows {
		if r.Table == "platform_hosts" && r.ID == id && r.Desired() {
			host, target, found = r.Host, r.Target, true
			break
		}
	}
	if !found {
		return reconcile.Result{Error: "active system host not found"}, errors.New("system host not found")
	}
	res, err := v.engine.PinStaticBlock(ctx, host, target)
	if err != nil {
		return res, err
	}
	if derr := conn.DeleteSystemHost(ctx, id); derr != nil {
		res.Error = "pinned + reloaded, but removing the DB row failed — delete this row so a reconcile doesn't conflict with the new static block: " + derr.Error()
	}
	return res, nil
}

// UnpinRoute converts a "Pinned · unmanaged" static block back into a managed
// platform_hosts row. Engine-first: the block is removed + folder file rendered +
// validated + reloaded (the host now serves from an orphan folder file), THEN the
// DB row is created to adopt it. REFUSED on protected domains by the engine. If the
// DB create fails the host still serves (as an orphan folder file) — surfaced so the
// operator re-adds it under System. GATED.
func (v *VhostEngineService) UnpinRoute(ctx context.Context, host string) (reconcile.Result, error) {
	if !v.LiveReloadEnabled() {
		return reconcile.Result{Error: liveGateMsg}, errLiveGate
	}
	res, target, err := v.engine.UnpinStaticBlock(ctx, host)
	if err != nil {
		return res, err
	}
	conn, err := v.openDB(ctx)
	if err != nil {
		res.Error = "unpinned + reloaded, but opening the DB to adopt the route failed (it now serves as an orphan folder file — re-add it under System): " + friendlyDBError(err)
		return res, nil
	}
	defer conn.Close()
	in, verr := caddydb.ValidateSystemHost(caddydb.SystemHostInput{Host: host, Target: target, IsActive: true}, v.guard())
	if verr != nil {
		res.Error = "unpinned + reloaded, but the adopted route is invalid (" + verr.Error() + ") — re-add it under System"
		return res, nil
	}
	if _, cerr := conn.CreateSystemHost(ctx, in); cerr != nil {
		res.Error = "unpinned + reloaded, but creating the DB row failed (" + cerr.Error() + ") — the host serves as an orphan folder file; re-add it under System"
	}
	return res, nil
}

// SaveSystemHost creates (ID==0) or updates a platform_hosts row, and persists its
// panel-local response headers (server-panel's own store, keyed by host). The row +
// headers become live on the next Reconcile. Headers are validated up front so a bad
// value rejects the whole save before any write.
func (v *VhostEngineService) SaveSystemHost(ctx context.Context, f SystemHostForm) error {
	cleanHeaders, herr := SanitizeHeaders(f.Headers)
	if herr != nil {
		return herr
	}
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
	var oldHost string
	if f.ID != 0 {
		oldHost, _ = conn.SystemHostByID(ctx, f.ID)
	}
	if f.ID == 0 {
		_, err = conn.CreateSystemHost(ctx, in)
	} else {
		err = conn.UpdateSystemHost(ctx, f.ID, in)
	}
	if err != nil {
		return err
	}
	if herr := v.settings.SetVhostHeaders(in.Host, cleanHeaders); herr != nil {
		return herr
	}
	// On a host rename, drop the old key so stale headers don't linger.
	if oldHost != "" && normalizeHostKey(oldHost) != normalizeHostKey(in.Host) {
		_ = v.settings.DeleteVhostHeaders(oldHost)
	}
	return nil
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
	ds, ok := v.sources.ActiveSource()
	if !ok {
		return nil, errors.New("no active data source — add one under Settings → Data Sources")
	}
	adapter, ok := adapterFor(ds.Engine)
	if !ok {
		return nil, fmt.Errorf("unsupported engine for data source %q", ds.Name)
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
