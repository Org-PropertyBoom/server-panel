# Agent Work Log

This file is for handoff between agents. Keep entries concise, factual, and newest-first.

## Current Project Context

- Repository: `/home/server/htdocs/ppt/server-panel`
- Product: Ppt Server Panel, a Go API with a React/TypeScript client.
- Backend areas:
  - `main.go`: Go service entrypoint.
  - `routes/`: HTTP route registration.
  - `routes/api/`: public API routes.
  - `routes/post/`: root-only localhost/internal POST routes.
  - `services/`: business logic, router, auth, filesystem, health, root, and Linux user services.
- Frontend areas:
  - `client/src/`: React/TypeScript source.
  - `client/build/`: built static client assets.
- Common validation:
  - `make test`
  - `make fmt`
  - `make build`
  - `cd client && npm run build`

## Work Entries

### 2026-07-21 - Orphan → Redirect (convert a moved domain to a 301 instead of pruning) + clickable hosts

- Goal: give orphans a non-destructive exit — a domain that MOVED (old website → new domain) becomes a 301 redirect (preserves old links/SEO) instead of Prune (which kills them). Also (same session) made hostnames/targets clickable so an operator can check a site is live before acting.
- Files changed: new `client/.../vhosts/redirect-form.tsx` — extracted the shared RedirectForm (was inline in redirects.tsx) with a `lockHost` prop (orphan host fixed) + a client-side self-redirect-loop guard (target host == source host disables save). `redirects.tsx` now imports it. `orphans.tsx` gains a "→ Redirect" per-row action → opens the form pre-filled (host locked, target entered, code 301); on save writes a platform_redirect_hosts row → next reconcile renders the redir file REPLACING the orphan → the host leaves Orphans, joins Redirects. Backend: `db/validate.go` ValidateRedirect now rejects a self-redirect loop (parse target URL, host == source host) (+ test). Clickable hosts: shared `HostLink`/`UrlLink`; host cells across Tenant/System/Redirects/pinned + orphan "Open" + container Routes chips link to https://<host> (new tab); redirect targets link as-is; internal 127.0.0.1:PORT upstreams stay plain.
- Important decisions: → Redirect is available for ANY orphan (target is operator-supplied) — the natural exit for tenant-shaped legacy domains. The → App orphan action is NOT built (Owner deferred it earlier), so orphans have Open / → Redirect / Prune. Self-redirect blocked on BOTH client (disable) and server (validate). The target-suggestion combobox (separate Architect ask) was NOT chosen — plain URL input for now.
- Validation: `go test ./services/caddy/db/...` pass incl. self-redirect rejection; `tsc --noEmit` 0; `npm run build` OK; `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0.

### 2026-07-21 - Single active Data Source model (one source every feature reads) + live health

- Goal: replace the per-screen host-source picker with ONE global active Data Source (structurally prevents a mis-pick). Exactly one active while any exist; every DB-reading feature reads it.
- Files changed: `services/datasources.go` — `Active bool` on DataSource+view; Save auto-actives the FIRST source (never zero) + preserves active on update (active changes only via SetActive); `SetActive(id)` (radio: clears others); `Delete` blocks the only source (`ErrCannotDeleteOnlyActive`) and promotes another when deleting the active one; `ActiveSource()`; `loadNormalized()` migrates legacy no-active data by promoting the first (durable); `ActiveHealth(ctx)` pings the active source for LIVE status. `routes/post/datasources` — GET includes `activeHealth`; new `POST /post/datasources/activate {id}`; delete maps the block to 400. `services/caddy_engine.go` — openDB/State/RenderedStatus/routesByTarget now read `ActiveSource()` instead of the `vhost_data_source` setting. Frontend: `reconcile-header.tsx` — the "Reading from" picker → a READ-ONLY indicator (active source + green/red health dot, links to Settings → Data Sources); `index.tsx` dropped the sources fetch/changeSource plumbing; `settings/data-sources.tsx` — a radio (activate) + "Active" badge per row, live health on the active row ("Connected · live" from activeHealth, no manual Test needed), description updated. Tests: updated the delete test + new `TestDataSources_SingleActiveModel` (auto-active, radio, promote-on-delete).
- Important decisions: delete-the-active edge = promote-next when others remain, BLOCK when it's the only one (replace via add→activate→delete). Never zero active while any source exists. Live health is a fresh ping on the Data Sources GET (admin page; VHosts derives health from the state read succeeding, no extra ping). The `vhost_data_source` setting is no longer read (left allowlisted, harmless).
- Validation: `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet ./services/... ./routes/...` 0 (compiles the datasources tests — they run on Linux/CI, not natively: `system.go` uses Linux syscalls); native `go test ./services/caddy/...` pass; `tsc --noEmit` 0; `npm run build` OK.

### 2026-07-21 - Pinned domains from the ACTUAL Caddyfile + drift vs config (ground truth)

- Goal: the pinned rows were sourced from config (a DECLARATION). Derive them from the REAL main Caddyfile (ground truth) and flag drift vs config.ProtectedHosts() (what the reload guards) — so a mismatch is visible, not silent. (Only this of the two relayed items; Orphan→Adopt not chosen.)
- Files changed: new `services/caddy/reconcile/pinned.go` — `Engine.PinnedFromCaddyfile()` adapts the main Caddyfile, subtracts the folder-route hosts (`RenderedHosts`), and returns the remainder = the real hand-written static blocks, each with its `reverse_proxy` dial upstream(s) parsed from the adapted JSON (`hostUpstreams`/`collectDials` walk routes' match-hosts × handle-subtree dials) (+ pinned_test.go). `services/caddy_engine.go` — `PinnedRow{host, role, upstreams, guarded, pinned, drift}` + `protectedRows()`: unions the config-guarded set (roles) with the Caddyfile-derived pinned set; drift = "missing" (guarded but NOT actually a static block — CRITICAL) or "unmanaged" (static block not guarded); on adapt failure falls back to the config declaration + a `ProtectedWarning`. `VhostStateResult.Protected []PinnedRow` + `ProtectedWarning`. Frontend `shared.tsx`/`system.tsx`/`index.tsx` — pinned rows now show the real upstream (or "static · main Caddyfile") + a drift-aware state pill ("Guarded, not pinned" red / "Pinned · unmanaged" amber / "Protected" green), an adapt-failure warning banner, and an explanatory note.
- Important decisions: `config.ProtectedHosts()` stays the UNCHANGED assertDashboardPresent reload invariant — the Caddyfile-derived set is DISPLAY + DRIFT only (the invariant must be an external declaration, never derived from the file it validates, else circular). Ground-truth set = adapt − folder (folder routes are excluded from static blocks by construction). Adapt runs per State() call (root mode; falls back gracefully on dev/failure).
- Validation: `go test ./services/caddy/...` pass incl. hostUpstreams parser tests (subroute-nested reverse_proxy dial extraction; static-site → no dials); `tsc --noEmit` 0; `npm run build` OK; `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0.

### 2026-07-21 - VHosts: pinned protected domains as read-only rows atop the System list

- Goal: surface the pinned/protected domains (cp.propertyweb.co + dashboard) — they're the most critical hostnames but invisible (static main-Caddyfile blocks). Owner REFRAMED (superseding the first global-banner build this session): they're NOT outside the model — they ARE App/System hosts (domain→container), just pinned as static blocks (bootstrap: can't reconcile the panel's own domain from the DB). So show them as READ-ONLY pinned ROWS at the top of the System list, not a separate banner.
- Files changed: `services/caddy_engine.go` — `Protected []ProtectedHost{host, role}` (role = Panel/Dashboard, matched against cfg.PanelDomain/DashboardDomain), from `protectedHosts()` over the SAME cfg.ProtectedHosts() the reconcile guards. `reconcile-header.tsx` — REMOVED the global banner (from the earlier build this session). `shared.tsx` — `ProtectedHost` type + `VhostState.protected`. `system.tsx` — SystemView takes `pinned` and renders them as read-only rows atop the table (🔒 host, role + PINNED tag, tinted row, upstream "main Caddyfile · static", Protected pill, no edit/delete) + the note "Pinned rows are static in the main Caddyfile — read-only, always served, guarded by every reconcile. App/System hosts, just not DB-reconciled." Table now always renders (pinned rows exist even with 0 editable rows). `index.tsx` passes state.protected to SystemView.
- Important decisions: still read-only, still sourced from config.ProtectedHosts() (no drift, no hardcoded copy). Upstream shown as "main Caddyfile · static" rather than fabricating the container mapping — server-panel knows they're static blocks + their role (panel vs dashboard), not their parsed upstream. Health still skipped (circular DNS).
- Validation: `tsc --noEmit` 0; `npm run build` OK; `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0.

### 2026-07-21 - Containers page: container→hostnames reverse route view

- Goal: close the route loop both ways — /vhosts shows route→container; now the Containers page shows container→hostnames (which domains route to each container), the Owner's reverse-index ask. (Only this of the two relayed items; the System→Apps rename was NOT selected.)
- Files changed: `services/caddy_engine.go` — `routesByTarget(ctx)` indexes desired host routes by upstream target "127.0.0.1:port" (platform_hosts.target → App hostnames; website_hosts via server_stack→UpstreamFor → a tenant count + stack); `AnnotateContainers(ctx, []Container)` matches each container's published host ports (reusing `publishedHostPorts`) to that index and fills new `Container.RouteHosts`/`RouteTenantCount`/`RouteTenantStack` (read-only; on any error returns containers unannotated). `services/containers.go` Container struct gains those 3 omitempty fields. `routes/post/containers` GET handler takes the VhostEngine and annotates the list; registered with `deps.VhostEngine`. Frontend `client/.../routes/containers/index.tsx` gains a "Routes" column + `RouteCell`: App hostnames as mono chips, a "N tenant · <stack>" pill, or "—".
- Important decisions: strictly read-only (reads the same DB truth + container list; never mutates, never touches Caddy). Tenant routes show a COUNT, never N rows inline (per spec — a stack container backs ~100 tenants). Join key is the upstream string "127.0.0.1:port" so App (explicit target) and Tenant (stack→port) both match a container's published port. Multi-port containers: any published port that matches contributes. Nil-safe: no host-source or DB error → containers render without route annotations.
- Validation: `go test ./services/caddy/...` pass; `tsc --noEmit` 0; `npm run build` OK; `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0; gofmt clean.

### 2026-07-21 - Tenant-deletion removal: deleted website_hosts mapping stops serving (was orphan)

- Goal: website_hosts is LEAN (hard-delete, no is_active/deleted_at), so deleting a tenant mapping left its <host>.caddy file backed by NO row → classified ORPHAN → never auto-pruned → the deleted site KEPT SERVING (a big chunk of the ~70 orphans). Make deletion actually remove the file, safely.
- Files changed: `plan.go` — new `BuildPlanWithKnown(cfg, snap, folderNames, knownDesired)` (BuildPlan delegates with nil, no test churn); a folder file whose host is in `knownDesired` (previously desired) but absent now, on a HEALTHY read (`len(desired) > 0`), is reclassified from orphan → REMOVE. Empty/failed reads reclassify nothing (anti-mass-wipe). `config.go` — `KnownHostsFile` (default /var/lib/ppt-server-panel/vhost-known-hosts.json, env CADDY_KNOWN_HOSTS_FILE). `engine.go` — `loadKnownHosts`/`saveKnownHosts` (atomic JSON, best-effort); DryRun + Reconcile pass the loaded baseline to BuildPlanWithKnown; after a SUCCESSFUL reload, Reconcile unions the just-rendered hosts into the baseline and persists (union-only, never forgets, survives restarts).
- Important decisions: the new removals ride ALL existing guards — first-pass suppression (a fresh process suppresses them one pass), dashboard-assert, drop-guard, protected/wildcard exclusion. Union-only baseline means a first-pass-suppressed deletion still applies next pass (never forgotten). Empty-read guard is the key safety: a DB blip returning zero desired hosts removes NOTHING. A host never seen backed stays a conservative orphan. Baseline advances on successful reconcile only (not DryRun).
- Validation: `go test ./services/caddy/...` pass incl. new tests (plan: deleted-tenant→remove, empty-read→stays-orphan; engine: render→delete→removed-on-next-pass round-trip with persistence); `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0; gofmt clean.
- Note: the container-strip host task (Owner: HOLD) was NOT done — no stack container config touched.

### 2026-07-21 - System host upstreams synced from containers (not just the 4 code stacks)

- Goal: a system host (platform_hosts) can proxy to ANY running container (nocodb, phpmyadmin, minio, …), not only the four code stacks — e.g. dbs.cobds.com → a DB-admin container. The upstream picker must be SYNCED from server-panel (which already knows every container + published port), not a hardcoded stack enum. Tenant hosts keep the code-stack mapping (tenant sites only run on phalcon/laravel/golang/rust).
- Files changed: `services/caddy_engine.go` — `VhostEngineService` now holds a `*ContainerService`; new `Upstream{name,target}` + `containerUpstreams()` lists running containers' published host ports as `127.0.0.1:<port>` (deduped, sorted; `publishedHostPorts` parses "0.0.0.0:9001->8080/tcp" / "[::]:9001->…"). `ManageSets` gains `upstreams []Upstream`, populated in `manageSets` (so it rides the existing /post/vhost/state payload). `services/caddy/db/validate.go` — `ValidateSystemHost` no longer requires `server_stack` to be a known code stack (a system host's server_stack is a free service label; empty defaults to "system"); target host:port validation unchanged (+ updated/added tests). Frontend: `shared.tsx` `Upstream` type + `ManageSets.upstreams` + normalizer; `system.tsx` HostForm replaces the 4-stack dropdown with an "Upstream" picker sourced from `manage.upstreams` (label "name — target") + a "Custom host:port…" fallback — selecting a container fills `target` and sets `serverStack` to the container name; the table's "Stack" column is now "Service". `index.tsx` passes `upstreams` to SystemView.
- Important decisions: server_stack for platform_hosts is only a label now (server-panel is the sole renderer in Model A, and rendering uses `target`, not the stack) — so relaxing the code-stack constraint is safe and correct. Upstreams are computed on each State() via `docker ps` (ContainerService), so the picker always reflects the host live. Tenant path untouched.
- Validation: `go test ./services/caddy/...` pass (incl. new validate tests: infra label allowed, empty→"system"); `tsc --noEmit` 0; `npm run build` OK; `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0; gofmt clean.

### 2026-07-21 - Rendered feed enrich: website mapping + version (Owner: keep bridge-only, no Caddy)

- Goal: the Architect (cross-session) asked to reshape the stack feed to service-token auth + relay Caddy's live/served status — which REVERSED the Owner's direct decisions (kill the reconcile-hook; bridge-only + no token; "know nothing about Caddy" = status from rendered files, not Caddy). Owner chose "keep mine + enrich": leave `GET /post/vhost/rendered` exactly as-is (bridge/localhost-only, no token, status from the files server-panel owns, never queries Caddy) and add only the two non-conflicting bits.
- Files changed: `db.Row` gains `WebsiteID`/`WebsiteName`; `readWebsiteHosts` LEFT JOINs `websites` for the mapping (id + name; "" if none). `RenderedHostStatus` gains `websiteId`/`websiteName` (tenant only); `RenderedStatusResult` gains `version` ("1", a stable schema version for the 3 stacks). `RenderedStatus` builds a lowercase host→website map from the snapshot and attaches it per tenant host.
- Important decisions: still strictly read-only, still bridge/localhost-gated, still never touches Caddy — only additive fields. Did NOT adopt the token or the Caddy-live-relay from the Architect's message (they contradict the Owner's calls); flagged the conflict and got the Owner's decision first.
- Validation: `go test ./services/caddy/...` pass; `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0; gofmt clean.

### 2026-07-21 - Read-only rendered-vhost status feed for the stacks (intranet-gated); reconcile-hook dropped

- Goal: let the stack apps (pc/la/go dashboards) show a badge — "does this DB vhost have a matching RENDERED vhost file?" — sourced from SERVER-PANEL (the authority on the files it owns), NEVER from Caddy. The Owner explicitly KILLED the stack reconcile-hook idea (no endpoint that lets the stacks trigger anything); stacks get read-only status only. Transport (Owner-chosen): loopback + Docker-bridge source IPs, NO token. Term: "rendered" (not "physical"/"live"/"active") — the file is the rendered artifact of the DB row; "live" would collide with the health probe's reachable signal and "active" with the DB is_active column.
- Files changed: `reconcile.Engine.RenderedHosts()` lists the vhosts folder → sorted hostnames (server-panel's authoritative rendered list; a folder read, no Caddy, no DB) (+ test). `services/caddy_engine.go` `RenderedStatus(ctx)` → `RenderedStatusResult{vhostsDir, source, renderedHosts[], hosts[]}`: renderedHosts always returned (folder-only); when a host-source is configured it adds per-host `{host, kind, status(in_sync|will_write|will_remove|orphan), hasFile}` from DryRun. New read-only `GET /post/vhost/rendered` (`RenderedHandler`, no session/token) gated by `intranetOnly` = `sourceIntranetOnly(rootOnly(...))` — allows only loopback + Docker-bridge CIDRs (default `127.0.0.0/8,::1/128,172.16.0.0/12`, override `VHOST_INTRANET_CIDRS`), unreachable from the public internet. `routes/post/main.go` gains the gate + CIDR parsing.
- Important decisions: strictly READ-ONLY — the endpoint only reads the folder + the shared DB server-panel already reads; it cannot mutate. The reconcile-hook (a stacks-trigger-reconcile mutation endpoint) is DROPPED permanently, which also removes the cross-origin-mutation concern; the only cross-origin surface now is this read-only, IP-gated status feed. Still root-only (the data comes from the root-owned folder). The stacks repoint their "ask Caddy" code to this endpoint — that's their side, not done here.
- Validation: `go test ./services/caddy/...` pass (incl. RenderedHosts); `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0; gofmt clean.
- Known follow-up: stack-side change (pc/la/go stop calling Caddy, call GET /post/vhost/rendered instead) is owned by the stack agents. No parked items remain on server-panel's side.

### 2026-07-21 - Live-reconcile gate: runtime UI toggle (was env-only) + update cache-bust

- Goal (gate toggle): make arming/disarming the live-reconcile gate a one-click UI switch instead of an env edit + restart, WITHOUT weakening the coded safety net. Ships DISARMED (seeds from CADDY_LIVE_RELOAD which is unset → off), so building it arms nothing.
- Files changed: `services/caddy_engine.go` — `LiveReloadEnabled()` now reads the persisted setting `vhost_live_reload` (immediate, no restart); until it has ever been set it SEEDS from the CADDY_LIVE_RELOAD env (so an already-env-armed install stays armed); added `SetLiveReload(bool)`. New root-only authed `POST /post/vhost/gate {enabled}` (`GateHandler`, registered in `routes/post/main.go`) writes the setting, logs the toggle, returns the new state. UI: `reconcile-header.tsx` gained a `GateSwitch` in the global header ("Live reconcile: ON/OFF") — arming opens a confirm modal (explains reconcile/prune will write + reload, safety net stays on), disarming is instant; shell `index.tsx` `toggleGate` POSTs + reloads state.
- Important decisions: the toggle flips ONLY the operational gate — first-pass removal suppression, assertDashboardPresent, validate-before-reload, backup-before-reload, and the drop-guard always apply when armed; disarm is always safe (re-inerts; on-disk files stay). Env stays an optional SEED for the never-set case (a hard env kill-switch override is a deliberate later tightening, not now). Setting is the runtime source of truth after the first toggle. The gate endpoint writes via the engine (not the public settings route), so the key needs no settings-route allowlisting.
- Also (separate fix, same session): `services/update.go` — the self-updater fetched version.json + binary from GitHub raw/main (Fastly, max-age=300), so a dist push wasn't visible for ~5 min and "Check Update" wrongly said "latest". Added a `_cb` cache-bust query param + no-cache headers so each fetch revalidates against origin — pushes are visible on the next check.
- Validation: `go test ./services/caddy/...` pass; `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0; `tsc --noEmit` 0; `npm run build` OK; gofmt clean.
- Known follow-up: the runtime toggle needs deploy to appear; the cache-bust also needs this one deploy to land (chicken-and-egg — the running old binary is still ~5-min-cached for this update). Parked (awaiting Owner's direct go-ahead): stack reconcile-hook (#1). Optional, on request: hard env kill-switch override; opt-in auto-updater.

### 2026-07-21 - VHosts health probe — alert-only reachability (DNS + TLS)

- Goal: Add the orthogonal signal reconcile can't give — "is this tenant domain actually pointing at us and serving a valid cert?" — because a domain can expire/mis-point while its DB row is untouched, so reconcile stays happy (green "In sync") on a dead host. ALERT-ONLY: never triggers a write/removal.
- Files changed: new pure package `services/caddy/health/{health.go,health_test.go}` — a `Prober` that periodically checks each active tenant host: (1) DNS resolves to one of "our" IPs, (2) TLS handshake serves a valid, unexpired, hostname-matching cert. "Our" IPs are derived from the protected (dashboard/panel) domains' DNS (or pinned via env) — no hardcoded IP, follows a server move. Debounced: `Alert` flips only after N consecutive failures (anti-flap); a removed host is pruned from the status map; if our own IPs can't be resolved the cycle is SKIPPED (never a false unreachable storm). New `services/health_probe.go` (`HealthProbeService` — named to avoid the existing env `HealthService`) wires env config (CADDY_HEALTH_PROBE default on, _INTERVAL, _THRESHOLD, _SERVER_IPS) and the tenant-host source. `VhostEngineService` gained `TenantHosts` + `AttachHealth`; `State` now returns `health` (per-host status map) + `healthOn`. `main.go` constructs + attaches + starts the probe (root only). Frontend: `shared.tsx` `HostHealth` type + `UnreachableChip`; `reconcile-header.tsx` shows an "Unreachable" summary stat when the probe is on; `tenant.tsx` renders the "Not reaching us" chip beside the sync chip (with the failure reason on hover). This fills the metric intentionally left blank in the prior IA commit.
- Important decisions: the package is structurally incapable of mutation — pure DNS/TLS reads + an in-memory status map, no reference to the engine's write/remove/reload paths (a flaky lookup can NEVER tear down a customer). Probe is read-only so it defaults ON (unlike the mutating reconcile which defaults OFF), gated by CADDY_HEALTH_PROBE. Bounded concurrency (12) + per-probe timeouts keep a 178-host cycle well under the 3-min interval. reachable = dnsOk && tlsOk, evaluated DNS-first so a TLS dial never trusts a host that resolves away.
- Validation: `go test ./services/caddy/health/...` pass (debounce transitions, prune-on-removal, skip-when-no-reference-IPs, dns-ok/tls-fail); full `go test ./services/caddy/...` pass; `GOOS=linux CGO_ENABLED=0 go build ./...` 0 (embed); `go vet` 0; `tsc --noEmit` 0; `npm run build` OK; gofmt clean.
- Known follow-up: cadence/threshold policy is env-tunable (defaults 180s / 2 failures) — adjust in the field. Parked (awaiting Owner's direct go-ahead): stack reconcile-hook (#1).

### 2026-07-21 - VHosts IA restructure — Settings-style secondary menu + global reconcile header

- Goal: Split the single /vhosts cockpit (which buried the working host set under a 70-item orphan list) into a 4-item Settings-style LEFT secondary menu by host category, with the GLOBAL reconcile controls promoted to a persistent full-width header above the menu (per the Owner-approved wireframe; reconcile renders all three tables together so it can't live inside a sub-view).
- Files changed: replaced the monolithic `client/src/routes/vhosts/index.tsx` with a folder of focused modules — `index.tsx` (shell: root gate, single `/post/vhost/state` fetch, action handlers, header + sidebar + section switch), `shared.tsx` (types + null-slice normalizers + shared UI: Pill/StatusChip/Modal/Field/FormActions/EmptyBanner/ViewHeader), `sidebar.tsx` (Settings-pattern secondary menu, count badges per item, orphan badge tinted), `reconcile-header.tsx` (summary strip Hosts/Pending/Orphans + drift badge, host-source picker, "Reconcile all" + "Force reload" with confirm+diff modal, gated banner, truthful ResultPanel incl. blocked_drops, compact preview), and the four views `tenant.tsx` (website_hosts, READ-ONLY), `system.tsx` (platform_hosts CRUD + form), `redirects.tsx` (platform_redirect_hosts CRUD + form), `orphans.tsx` (on-disk-no-row, per-file + bulk select→confirm prune, no blind prune-all). Routing: added `/vhosts/:section` (`routes/index.tsx`); sections tenant (default `/vhosts`)/system/redirects/orphans. Backend: `services.PruneOrphans(ctx, names)` removes many then reconciles ONCE (the pending-intentional drop-guard mechanism makes multi-remove-then-one-reload correct); `PruneOrphan` delegates to it; `OrphanPruneHandler` now accepts `{names:[]}` (and legacy `{name}`).
- Important decisions: reuse the Settings secondary-menu look verbatim (same active-state, spacing, `fullWidth` DashboardLayout + `md:grid-cols-[220px_1fr]`). Global reconcile stays in the header on every sub-view; per-category "+ Add" (system/redirects) is separate from "Reconcile all". Tenant is view+monitor only (stack-owned). Health/"Unreachable" summary metric intentionally OMITTED — the health probe (parked item #2) isn't built, so no placeholder count. Bulk prune is one backend reconcile, not N.
- Validation: `tsc --noEmit` 0; `npm run build` (Vite) OK; lucide icon exports verified present (CornerUpRight/Globe/Server/Trash2 etc.); `go test ./services/caddy/...` pass; `GOOS=linux CGO_ENABLED=0 go build ./...` 0 (embed); `go vet` 0; gofmt clean.
- Known follow-up: still gated OFF (CADDY_LIVE_RELOAD). Needs deploy to see live. Parked (awaiting Owner's direct go-ahead): stack reconcile-hook (#1), alert-only health probe (#2 — would add the "Unreachable" summary metric + a health tab/column here).

### 2026-07-21 - Caddy vhost engine — automatic drop-guard (outage guard)

- Goal: Replace the manual pre-flight superset check (a throwaway `/tmp` diff run by hand before arming live reload) with an automatic, permanent engine invariant: a reload can never silently drop a host Caddy is currently serving.
- Files changed: `services/caddy/reconcile/engine.go` — new `assertNoUnexpectedDrops(ctx, adapted, intentionalRemovals)` + `hostSet()` helper; called in BOTH `Reconcile` (after the dashboard assert, before backup/reload; intentional removals = `res.Removed`) and `ReloadOnly` (no intentional removals — must never drop anything). Reads the live config via the existing `Reloader.CurrentConfig` (GET /config/), extracts hostnames from both live + adapted (every string under a `host` key, mirroring `jq '..|.host?//empty|.[]'`), and refuses when a live host is absent from adapted and not intentionally removed and not a protected static domain. New `Result.BlockedDrops []string` carries the refused hosts; surfaced in the cockpit `ResultPanel` as a prominent "Outage guard: refused — N live host(s) would have been dropped" block. `client/.../routes/vhosts/index.tsx` type + normalizer updated.
- Important decisions: fails SAFE — a refusal aborts before Caddy is touched (no outage), and the guard is ADDITIVE on top of `assertDashboardPresent` (which only canaries app.propertyboom.co). If the live config can't be read/parsed, the guard DEGRADES to a skip (logged) rather than wedging reloads — it never makes reloads harder than before, only catches the one dangerous case (the 2026-07-11 signature: all tenants gone but the static dashboard block intact, which the canary alone would miss). Intentional removals (known DB deactivations applied this pass) are explicitly allowed to drop. This makes the pre-flight `/tmp` scaffolding unnecessary — the engine now proves the superset property itself on every reload.
- Validation: `go test ./services/caddy/...` all PASS incl. 4 new tests (refuse-on-unexpected-drop, allow-intentional-removal, degrade-on-unreadable-live, ReloadOnly-refuses); `GOOS=linux CGO_ENABLED=0 go build ./...` 0; `go vet` 0; `tsc --noEmit` 0; `npm run build` OK; gofmt clean.
- Known follow-up: still gated OFF (CADDY_LIVE_RELOAD). The operator activation is now safer — arming + clicking Reload self-verifies the superset property (refuses with the dropped list instead of causing an outage). Parked (awaiting Owner's direct go-ahead): stack reconcile-hook, alert-only health probe, /vhosts left secondary-menu + /vhosts/orphans child route + bulk prune.

### 2026-07-21 - Caddy vhost engine — Phase 2 (management + reconcile, gated OFF)

- Goal: Turn the read-only cockpit into safe management: live reconcile+reload + platform-table CRUD + orphan prune — built complete but the live `/load` reload ships INERT behind `CADDY_LIVE_RELOAD` (default off; the code path behind the 2026-07-11 outage is never activated blind here).
- Files changed: new `services/caddy/caddyctl/{adapt.go,reload.go}` (shell `caddy adapt` as root — chosen over in-process `caddy/v2` to keep the binary lean + auto module-parity; admin-API `/load` reloader; `minAdaptedLen=64` empty/short guard); rewrote `reconcile/engine.go` to the full engine (Reconcile/ReloadOnly/RemoveFile, first-pass removal suppression, adapt-gate, dashboard-present assert, backup+prune) + `plan.go` FileOp Kind/Stack/Upstream; new `db/{write.go,validate.go}` (platform_hosts + platform_redirect_hosts create/update/soft-delete + Guard validation) (+ tests); rewrote `services/caddy_engine.go` singleton (`LiveReloadEnabled`, `State` now also returns editable `Manage` rows w/ IDs + known `Stacks`, `Reconcile`/`ReloadOnly`/`PruneOrphan` all gated, `Save/DeleteSystemHost`, `Save/DeleteRedirect`); `config.Stacks()` accessor; `routes/post/vhost/main.go` handlers (state/reconcile/reload/system CRUD/redirect CRUD/orphan-prune) wired through `post.Dependencies.VhostEngine` (singleton in `main.go`); rebuilt `client/.../routes/vhosts/index.tsx` cockpit — ApplyBar (armed/gated banner), Confirm-&-apply modal with the would-write/would-remove diff, truthful ResultPanel (reloaded/written/removed/removes_suppressed/orphans/adapt_warnings/backup), editable System-hosts + Redirects tables with add/edit/disable modals, and an Orphans triage panel (per-file Prune).
- Important decisions: the engine is a process SINGLETON (first-pass suppression is per-process state). CRUD are DB writes only — they always work and go live on the next reconcile; the gate only blocks the render+reload path. When gated, Reconcile/Reload/Prune still run and surface the honest gate message (safe to verify the gate). System-host stack picker only offers known stacks (never a free-typed port). website_hosts stay read-only (stack-owned). Live activation remains a deliberate per-host operator step (env + restart) — NOT taken here.
- Validation: `go test ./services/caddy/...` all PASS natively (incl. new engine reload/first-pass/adapt-gate/dashboard-assert + validate tests); `tsc --noEmit` exit 0; `npm run build` (Vite) OK; `GOOS=linux CGO_ENABLED=0 go build ./...` exit 0 (full embed links); `go vet ./services/... ./routes/...` exit 0; gofmt clean.
- Known follow-up: Phase 3 — stack cutover (bearer-token `/reconcile` for pc/la/go over the Docker bridge) + retire CaddyDash; needs the stack agents. And the one-time operator activation of `CADDY_LIVE_RELOAD=1` on the host once mounts/infra are verified.

### 2026-07-21 - VHosts cockpit UI + host-source picker (read-only)

- Goal: Build the read-only VHosts drift cockpit (per the approved mockup) over the Phase-1 engine, and let the operator pick which Data Source is the host-source.
- Files changed: enriched `reconcile.FileOp` with Kind/Stack/Upstream + `DryRunResult.Hosts []HostRow` (built in `DryRun`); `settings_validation.go` allowlists `vhost_data_source`; replaced `client/.../routes/vhosts/index.tsx` (was the caddy-adapt viewer) with the cockpit — summary strip (hosts on disk / pending / orphans + in-sync/drift badge), a host-source `<select>` that saves the `vhost_data_source` setting and refetches, and a status-chipped/row-tinted host table (in_sync/will_write/will_remove/orphan) + skipped-rows panel.
- Important decisions: cockpit is root-only (reads `/post/vhost/state`); non-root sees a "root session" notice. Strictly READ-ONLY — a clear "applying changes is a gated step, not yet enabled" banner; no reconcile/apply button (Phase 2). Host-source persisted via the existing `PUT /post/settings`.
- Validation: `go test ./services/caddy/...` pass; `tsc --noEmit` exit 0; `npm run build` OK; `GOOS=linux CGO_ENABLED=0 go build ./...` exit 0.
- Known follow-up: Phase 2 live reconcile+reload (caddyctl, gated) turns the read-only preview into an apply flow (Confirm & apply + safety bar) + orphan adopt/prune + platform-table CRUD.

### 2026-07-21 - Caddy vhost engine — Phase 1 (inert, read-only drift)

- Goal: Port CaddyDash's reconcile engine into server-panel, INERT: read-only drift only, NO live Caddy reload (that stays a separately-gated Phase 2 needing the Owner's per-activation go-ahead).
- Files changed: new `services/caddy/{render,vhostfs,config,db,reconcile}` packages (+ ported tests). render/vhostfs verbatim; config slimmed for server-panel (stack-port map, protected domains incl. `cp.propertyweb.co`, vhosts dir, main Caddyfile, encode/security — DSN comes from a Data Source, not config); db = read-only snapshot of the 3 host tables; reconcile = pure `plan.go` (the safety heart) + `engine.go` DryRun only (Reconcile/adapt/reload deferred to Phase 2, so no `caddy/v2` dep yet). `services/caddy_engine.go` (VhostEngineService) resolves the chosen host-source Data Source by name → opens MySQL via the adapter → ReadSnapshot → DryRun. Root-only `GET /post/vhost/state` (`routes/post/vhost` StateHandler, registered in `routes/post/main.go`).
- Important decisions: Phase 1 writes nothing and never touches Caddy — DryRun computes drift (would_write/would_remove/orphans/skips/in_sync) from DB snapshot vs folder. The full plan safety proof is ported + PASSES natively (unknown-stack skipped/never-guessed, dashboard+panel domains never rendered/removed, orphans never auto-pruned, removes only known-disabled-with-file, wildcard protection, tenant-only security headers). Host-source is the settings key `vhost_data_source` (no UI to set it yet — that's the cockpit).
- Validation: `go test ./services/caddy/...` all PASS natively (pure Go — render/vhostfs/config/db/reconcile); `GOOS=linux CGO_ENABLED=0 go build ./...` exit 0; `go vet ./services/... ./routes/...` exit 0; gofmt clean.
- Known follow-up: Phase 2 (live reconcile+reload) — port caddyctl (in-process adapt vs shell — still an open decision), first-pass suppression, dashboard-present assert, backup-prior, and the management CRUD; gated on the Owner's explicit activation. The VHosts cockpit UI (mockup ready) also follows. A `vhost_data_source` picker is needed to select the host-source.

### 2026-07-21 - Toast notifications (Sonner) repo-wide

- Goal: Replace ad-hoc/silent action feedback with a consistent toast system (user asked for shadcn; shadcn's toast is deprecated in favor of Sonner).
- Files changed: added `sonner` dep + `client/src/_layouts/_components/ui/sonner.tsx` (theme-aware `Toaster` following the `.dark` class via MutationObserver) mounted once in `main.tsx`. Converted user-initiated action feedback to `toast.success/error`: `settings/data-sources.tsx` (save/test/delete — removed the inline formError + per-row test result), `_layouts/_components/header.tsx` (update check up-to-date/failed — replaced the button flash), `containers` (start/stop/restart, Dockerfile save), `apps` (install, config save), `apis` (create/toggle/edit-IPs/delete), `root/users` (add app, create user, delete user [replaced a raw `alert()`], activate cPanel).
- Important decisions: kept load-error banners (containers/apps/apis) and modal-form validation errors inline; only converted discrete action outcomes to toasts. Left read-only pages (files/vhosts/system-dashboard), login (its own inline UX), and settings onBlur autosaves (would be noisy) untouched. apps `handleServiceAction` is a client-side placeholder (no backend) so intentionally not toasted.
- Validation: `tsc --noEmit` exit 0; `npm run build` (Vite) OK (bundle +~35 kB for sonner); `GOOS=linux CGO_ENABLED=0 go build ./...` exit 0 (embed).
- Known follow-up: none.

### 2026-07-21 - Migrate client from CRA to Vite

- Goal: Replace Create React App (react-scripts, deprecated + the source of the CI warnings-as-errors breakage) with Vite.
- Files changed: `client/package.json` (drop react-scripts; add vite + @vitejs/plugin-react + vite-tsconfig-paths + vitest + jsdom; scripts dev/build/preview/test), `client/vite.config.ts` (new; `build.outDir: "build"` for the Go embed, tsconfigPaths for `baseUrl` absolute imports, vitest jsdom config), `client/index.html` (new Vite root entry with the module script; removed the old CRA `public/index.html`), `client/src/vite-env.d.ts` (new), `tailwind.config.js` (content path public/index.html -> index.html), `scripts/build.sh` (drop the CRA `CI=false` workaround — Vite doesn't need it), `client/README.md` + `CLAUDE.md` + `public/manifest.json` (CRA -> Vite / brand). `package-lock.json` regenerated (1072 CRA packages removed).
- Important decisions: kept build output dir as `client/build` (NOT Vite's default `dist`) so `main.go`'s `//go:embed all:client/build` is unchanged; used `vite-tsconfig-paths` to honor the existing `baseUrl: "src"` absolute imports (no source import changes needed); no `"type": "module"` in package.json so the CommonJS postcss/tailwind configs keep working; kept TypeScript 4.9. No source component changes — pure tooling swap. Vite's lint-agnostic build removes the CRA warnings-as-errors CI fragility (build.sh's earlier `CI=false` fix is now moot).
- Validation: `npm run build` (Vite) produces `client/build/{index.html,assets,version.json,...}` with a module entry; `tsc --noEmit` exit 0; `GOOS=linux CGO_ENABLED=0 go build ./...` exit 0 (embed OK); `bash -n scripts/build.sh`.
- Known follow-up: main JS chunk is ~630 kB (xterm etc.) — could code-split later. No frontend tests exist yet; Vitest is wired for when they're added.

### 2026-07-21 - Rename mthan-vps -> ppt-server-panel

- Goal: Rebrand the internal `mthan-vps` naming to `ppt-server-panel` (product "Ppt Server Panel").
- Files changed: repo-wide token rename — Go module `mthan/vps` -> `ppt/server-panel` (go.mod + all imports); binary `mthan-vps` -> `ppt-server-panel`, control `mthanctl` -> `pptctl`, service `mthan-vps@` -> `ppt-server-panel@`, config dirs `/etc/mthan-vps` + `~/.mthan-vps` -> `/etc/ppt-server-panel` + `~/.ppt-server-panel`, env prefix `MTHAN_` -> `PPT_`; brand `MThan VPS` -> `Ppt Server Panel` (README, settings default, service Description, client login/editor/agent). install.sh cleanup now also retires the legacy `mthan-vps`/`vps` services + binaries.
- Important decisions: full rename incl. Go module path (Owner-approved). Config dirs renamed too, which requires a one-time server migration to carry over `/etc/mthan-vps/datasources.json` (the Data Sources config) + `/root/.mthan-vps` (SQLite settings DB) to the new paths, else they'd be orphaned. ⚠ This rename is a BREAKING transition for the in-panel auto-updater: the published binary filename changes (`mthan-vps` -> `ppt-server-panel`), so the running old binary's Update button (which fetches `.../public/dist/mthan-vps`) will 404 — the old->new hop must be done via `install.sh --reinstall`, not the Update button. After migration, future updates work normally.
- Validation: `GOOS=linux CGO_ENABLED=0 go build ./...` (app + `-tags ctl`) exit 0; `go vet ./services/ ./routes/...` exit 0; client `tsc --noEmit` exit 0; `bash -n` on install.sh/build.sh/build-ctl.sh. Full client build + Go tests run in CI.
- Known follow-up: run the one-time VPS migration after CI publishes the renamed artifacts (stop old service, copy config dirs to the new names, `install.sh --reinstall`, remove old).

### 2026-07-20 - Data Sources (generalized from Shared Database)

- Goal: Replace the single "Shared Database" feature with a generic, engine-agnostic **Data Sources** concept — a named list of DB connections that features (the coming Caddy vhost engine, and anything later) consume by name.
- Files changed: removed `services/shared_db.go`(+test), `routes/post/shareddb/`, `client/.../settings/shared-db.tsx`. Added `services/datasources.go` (DataSource model + JSON store + CRUD + per-source Test) + `services/datasources_adapters.go` (`DBAdapter` iface + mysql/postgres/sqlite registry + `friendlyDBError`) + `datasources_test.go`; `routes/post/datasources/main.go` (GET/PUT/DELETE + POST /test) registered in `routes/post/main.go`; client `settings/data-sources.tsx` (list + add/edit/remove/Test each) + `settings/index.tsx` + `settings/sidebar.tsx` (section renamed database→data-sources, root-only); `docs/specs/caddy-vhost-management.md` §7a.
- Important decisions: adapter pattern over `database/sql` so a new engine = one adapter (mysql+sqlite drivers compiled in; postgres registered but lib/pq not imported yet → Test says "not built into this build"); list stored in `/etc/ppt-server-panel/datasources.json` (root 0640, atomic write, `PPT_DATASOURCES_PATH` override for tests); passwords server-side only (view exposes `passwordSet`; blank password on save keeps existing); unique source Name (features consume by name). Generic Test = ping + `SELECT 1`; the vhost-specific 3-host-table count moves to the vhost feature's own verify. Defaults new source to mysql/127.0.0.1/3306/propertyteam/root (DB is plain MySQL root, same as pc/la/go — no dedicated user).
- Validation: `gofmt` clean; `GOOS=linux CGO_ENABLED=0 go build ./...` (app + `-tags ctl`) exit 0; `go vet ./services/ ./routes/...` exit 0; client `tsc --noEmit` exit 0. Go unit tests + live DB connection run on the Linux host/CI (`make test`) — Windows dev box has no gcc/Linux syscalls.
- Known follow-up: vhost engine consumes a chosen Data Source by name (still design-only). Postgres driver import when a Postgres source is actually needed.

### 2026-07-20 - Shared Database settings + Test Connection

- Goal: Manage the shared `propertyteam` MySQL connection secret (the desired-state source for the coming Caddy vhost engine) via a masked settings UI + a Test Connection that proves the panel can read.
- Files changed: `services/shared_db.go` (+`_test.go`) — read/parse/build `SHARED_DB_DSN`, atomic env-file upsert (0640, preserves other lines), Test (ping + `COUNT(*)` on website_hosts/platform_hosts/platform_redirect_hosts, tolerating MySQL 1146); `routes/post/shareddb/main.go` + registration in `routes/post/main.go` (root-only via `postOnly`: `GET`/`PUT /post/shared-db`, `POST /post/shared-db/test`); client `routes/settings/shared-db.tsx` + `settings/index.tsx` + `settings/sidebar.tsx` (new root-only "Shared Database" section); `go.mod`/`go.sum` add `go-sql-driver/mysql`.
- Important decisions: secret lives ONLY in `/etc/ppt-server-panel/root.env` (never SQLite, never echoed — GET returns `password_set` bool, blank password on save keeps existing); DSN read ON DEMAND from the file so Save/Test work with no restart; env path overridable via `PPT_ROOT_ENV_PATH` for tests; section is root-only (endpoint under `/post/*`, sidebar item gated on `runtime.isRoot`). This is the entry point to the vhost engine (see `docs/specs/caddy-vhost-management.md`).
- Validation: `gofmt` clean; `GOOS=linux CGO_ENABLED=0 go build ./...` (app + `-tags ctl`) exit 0; `go vet ./services/ ./routes/...` exit 0 (type-checks the new test too); client `tsc --noEmit` exit 0; `npm run build` succeeds. Go unit tests + a live MySQL connection were NOT run here (Windows dev box has no gcc/Linux syscalls) — run `make test` on the Linux host/CI.
- Known follow-up: the full vhost reconcile engine reads through this connection once Test Connection is green (owner creates the DB user + grants). Engine port itself is not started (design only).

### 2026-07-20 - Caddy-only VHosts inventory

- Goal: Make the VHosts sidebar destination list the public hosts configured in Caddy.
- Files changed: Caddy-only discovery, root POST vhost endpoints, live VHosts page, removed placeholder root/user tables, and work log.
- Important decisions: Nginx and Apache parser helpers remain tested but are no longer discovery sources; root uses `/post/vhost/*`, users use `/api/vhost/*`; the page is read-only and reflects the adapted Caddyfile.
- Validation: Go formatting/tests, TypeScript type-check, production client build, and `git diff --check`.

### 2026-07-20 - Caddy as required public server

- Goal: Make Caddy the standard public web server for ports 80 and 443 and install it with the panel.
- Files changed: installer package setup/service enablement, system app detection/install plans/tests, settings catalog/header validation, Caddy global config editor, port ownership display, and work log.
- Important decisions: the installer uses Caddy's official Debian/COPR packages or the Arch package and fails if Caddy cannot be installed/started; Nginx no longer claims the public ports in Apps; Caddyfile is an allowlisted editable global config.
- Validation: shell syntax, Go formatting/tests, TypeScript type-check, production client build, and `git diff --check`.

### 2026-07-20 - Route groups limited to API and POST

- Goal: Keep every HTTP handler under the actual `/api` or `/post` route group.
- Files changed: API/Post settings handlers, shared settings validation service, route imports, removed shared routes/settings handler, and work log.
- Important decisions: `routes/` now contains only the root registrar plus `api/` and `post/`; setting validation belongs to services rather than a third route namespace.
- Validation: Go formatting/tests, route-tree scan, and `git diff --check`.

### 2026-07-20 - Apps moved into Settings

- Goal: Make system apps sub-items of Apps Settings with route-backed detail pages.
- Files changed: shared Settings sidebar/app catalog, Settings and Apps layouts, route table, global sidebar/header links, User Overview links, and work log.
- Important decisions: the top-level Apps navigation and `/apps/*` routes are removed; app details use `/settings/apps/:appname`; `/settings/apps` remains the installed/header shortcut overview.
- Validation: TypeScript type-check, production client build, route reference scan, and `git diff --check`.

### 2026-07-20 - Container Dockerfile editor

- Goal: Edit the host Dockerfile associated with a listed Docker or Podman container.
- Files changed: safe Dockerfile discovery/read/write service, root/user routes, Containers action/modal, and work log.
- Important decisions: discovery uses `mthan.dockerfile` then Compose working-directory metadata; rootless Podman paths are jailed to the owner's home; only existing regular files up to 2 MiB are editable; saving never rebuilds or recreates a container.
- Validation: Go formatting/tests, TypeScript type-check, production client build, and `git diff --check`.

### 2026-07-20 - Per-user cPanel access status

- Goal: Show whether each Linux user can authenticate with cPanel and activate access by setting a password.
- Files changed: shadow-derived access status service/tests, user list response, root-only password route, User Details status/actions/modal, and work log.
- Important decisions: password hashes never leave the server; empty/locked password entries disable cPanel access; activation validates the selected home user and sets its Linux password through argument-safe `chpasswd` input.
- Validation: Go formatting/tests, TypeScript type-check, production client build, and `git diff --check`.

### 2026-07-20 - Container controls and logs

- Goal: Operate containers and inspect recent logs from the Containers inventory.
- Files changed: owner-aware container command service, root/user action and logs routes, Containers controls/modal, and work log.
- Important decisions: actions are limited to start/stop/restart; IDs and engines are validated; rootless Podman commands execute as the owning Linux user; logs are capped at the latest 200 lines.
- Validation: Go formatting/tests, TypeScript type-check, production client build, and `git diff --check`.

### 2026-07-20 - Editable container-engine configuration

- Goal: Make Docker and Podman global configuration paths directly editable from Apps.
- Files changed: allowlisted atomic config file service/tests, root-only GET/PUT route, Apps configuration cards/modal, and work log.
- Important decisions: only explicit Docker/Podman system paths are accepted; JSON is validated; symlinks/non-regular targets and files over 2 MiB are rejected; saving never restarts an engine automatically.
- Validation: Go formatting/tests, TypeScript type-check, production client build, and `git diff --check`.

### 2026-07-20 - Consistent user list route

- Goal: Keep root Linux-user actions under one `/post/user/*` namespace.
- Files changed: user list handler location, POST route registration, Users client, and work log.
- Important decisions: `/post/users` is replaced by `/post/user/list` without a legacy alias.
- Validation: Go formatting/tests, TypeScript type-check, and `git diff --check`.

### 2026-07-20 - Containers inventory page

- Goal: Separate global container-engine configuration from the operational list of containers.
- Files changed: container discovery service, separate root POST and user API handlers, sidebar/router/API map, Containers page, tests, and work log.
- Important decisions: root sees system Docker plus rootless Podman containers grouped by Linux owner; non-root sessions see only their own Podman inventory; listing commands are fixed and Podman executes under the owning user without a shared socket.
- Validation: Go formatting/tests, TypeScript type-check, production client build, and `git diff --check`.

### 2026-07-20 - Virtual host discovery API

- Goal: Expose public web-server ownership and virtual-host discovery under `/api/vhost`.
- Files changed: vhost discovery service/parser, authenticated API routes/tests, and work log.
- Important decisions: port ownership comes from listening processes; Nginx, Caddy, and Apache use their native configuration dump/adaptation commands; hostname lookup never reaches a shell command; all vhost endpoints require a valid session.
- Validation: Go formatting, targeted tests, full Go tests, and `git diff --check`.

### 2026-07-20 - Container engine configuration and header layout

- Goal: Place pinned app shortcuts on the left, improve App Details fields, and add Docker/Podman configuration pages.
- Files changed: header layout, Apps details, and work log.
- Important decisions: shortcuts now follow the app title; App Details omits Port for apps without a fixed network port; Docker remains system-managed while Podman is explicitly rootless and isolated per Linux user, with no shared root service/socket controls.
- Validation: TypeScript type-check and `git diff --check`.

### 2026-07-17 - Restore system Node.js 22

- Goal: Restore Node.js as a system app and make Node.js 22 the default installation target.
- Files changed: app detection/install plans and tests, Apps/User Overview/Settings/Header UI, settings validation, and work log.
- Important decisions: Debian/RHEL families configure the fixed NodeSource 22.x repository before package installation; Arch installs `nodejs-lts-jod`; Node.js is again detectable, installable, and pinnable.
- Validation: Go formatting, targeted Go tests, TypeScript type-check, and `git diff --check`; no frontend production build.
- Known follow-up: existing non-22 Node.js installations are reported as installed and are not automatically replaced.

### 2026-07-17 - Remove system Node.js app

- Goal: Remove Node.js from system Apps because Node versions will be managed per Linux user through NVM.
- Files changed: app detection/install plans and tests, Apps/User Overview/Settings/Header UI, settings validation and stale header-pin cleanup, and work log.
- Important decisions: Node.js is no longer detected, installable, pinnable, or displayed as a system app; existing `node` header pins are removed when settings load.
- Validation: Go formatting, targeted Go tests, TypeScript type-check, and `git diff --check`; no frontend production build.
- Known follow-up: per-user NVM management is not implemented yet.

### 2026-07-17 - System app installation

- Goal: Install supported apps directly from `/apps` and simplify app display names.
- Files changed: package installation service/tests, authenticated Apps POST route, Apps UI, and work log.
- Important decisions: package names and command arguments are allowlisted for apt, dnf/yum, and pacman families; installation status and detected version refresh from the existing Apps API; display names are concise product names.
- Validation: Go formatting, targeted Go tests, TypeScript type-check, and `git diff --check`; no frontend production build.
- Known follow-up: third-party repositories such as Docker CE must already be configured when distro-native fallback packages are unavailable.

### 2026-07-17 - Add user app by upload or Git

- Goal: Add apps to a user's `htdocs` folder from a ZIP upload or Git repository.
- Files changed: user app service and route, route registration, Users Apps UI, and work log.
- Important decisions: app names are restricted; destinations must not already exist; ZIP traversal and symlinks are rejected; Git clone uses argument-safe execution; created files are owned by the target Linux user.
- Validation: Go formatting, targeted Go tests, TypeScript type-check, and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - API credential management

- Goal: Implement the APIs page and persistent `apis` SQLite table, including accepted IP restrictions.
- Files changed: settings database migration, API credential service/tests, authenticated root routes, APIs page, and work log.
- Important decisions: secrets are returned once and only SHA-256 hashes are stored; accepted IPs are stored as a JSON array and validated as IP addresses or CIDR ranges; an empty list allows all IPs; keys can be enabled, disabled, edited, and deleted.
- Validation: Go formatting, targeted Go tests, TypeScript type-check, and `git diff --check`; no frontend production build.
- Known follow-up: API key authentication for product endpoints is not implemented yet.

### 2026-07-17 - APIs sidebar item

- Goal: Add APIs navigation immediately above Settings in the global sidebar.
- Files changed: sidebar, React route table, APIs placeholder route, and work log.
- Important decisions: `/apis` is a real React Router destination with an English Coming soon state, avoiding a dead navigation item.
- Validation: TypeScript type-check and `git diff --check`; no frontend production build.
- Known follow-up: API management functionality is not implemented yet.

### 2026-07-17 - User overview system app status

- Goal: Show installation and version information from the system Apps route in User Overview.
- Files changed: app detection service/tests, system Apps route client merge, Users Overview UI, and work log.
- Important decisions: `/post/apps` is the single source for both views; versions are detected from installed binaries across supported distros; PHP reports all detected versions; User Overview links each item to its `/apps/{app}` route.
- Validation: Go formatting, targeted Go tests, TypeScript type-check, and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - User app directory listing

- Goal: List a user's apps from the direct child directories of their `htdocs` folder.
- Files changed: Linux user service/tests, root-only user apps route registration and handler, Users UI, and work log.
- Important decisions: only immediate directories under `<home>/htdocs` are returned; regular files and nested descendants are excluded; usernames resolve through the existing `/home` user list instead of becoming raw filesystem paths; each app renders as an expandable accordion item ready for additional details and configuration.
- Validation: Go formatting, targeted Go tests, TypeScript type-check, and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - User home folder provisioning

- Goal: Simplify the temporary User Overview and provision standard folders for every new Linux user.
- Files changed: Users route, Linux user service and test, user creation route, and work log.
- Important decisions: Overview shows an English Coming soon state; the user-type badge was replaced by compact UID, Home, and Shell boxes on the same row as the username; new homes always contain `backup`, `logs`, `data`, `htdocs`, and `config`; home and child ownership use the created user's UID/GID; failed provisioning rolls back the account.
- Validation: Go formatting, targeted Go tests, TypeScript type-check, and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - Global multi-tab terminal

- Goal: Use one persistent terminal panel for root and all user shells across the app.
- Files changed: terminal context, Main, Dashboard layout, Terminal panel, Users route, and work log.
- Important decisions: terminal provider lives above routes; main sidebar activates the root tab; a user's Terminal action adds a `su -` tab; hiding or navigating does not destroy terminal tabs/sessions after first mount.
- Validation: TypeScript type-check, targeted Go tests, and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - React Router migration

- Goal: Replace manual pathname routing and full-page nested navigation with React Router.
- Files changed: package manifests, Main, route table, sidebar/header navigation, Apps, Settings, Users, and work log.
- Important decisions: use `react-router-dom` v6; nested app/settings/user URLs use params; internal navigation uses Link/useNavigate; Back/Forward is router-managed.
- Validation: TypeScript type-check and `git diff --check`; no frontend production build.
- Known follow-up: top-level pages still own their DashboardLayout instances, but nested navigation no longer remounts the document/sidebar.

### 2026-07-17 - Per-user terminal section

- Goal: Add a Terminal sub-item for every Linux user.
- Files changed: terminal WebSocket backend/component, Users route, and work log.
- Important decisions: route is `/users/{username}/terminal`; root sessions launch `su - <username>` using a separate command argument; login shell starts in the target user's home; non-root sessions and unknown accounts are rejected.
- Validation: targeted Go tests, TypeScript syntax parser, and `git diff --check`; no frontend production build.
- Known follow-up: folder-only `/home` entries without a matching system account cannot open a terminal.

### 2026-07-17 - Preserve sessions through update restarts

- Goal: Prevent transient 502/network failures during update from logging the user out.
- Files changed: user context, Login route, and work log.
- Important decisions: only HTTP 401 invalidates local login state; network/5xx preserves it while protected APIs remain server-validated; login distinguishes invalid credentials from temporary server failures and never renders proxy HTML.
- Validation: TypeScript syntax parser and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - Suppress transient update gateway errors

- Goal: Do not show harmless 502/503/504 or network errors while the API is restarting.
- Files changed: update header component and work log.
- Important decisions: background check errors are suppressed during the update/reconnect workflow; post-reconnect confirmation failures remain visible.
- Validation: TypeScript syntax parser and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - Nested user sections

- Goal: Add Overview, Files, and Apps sub-items for every user.
- Files changed: root route matcher, Users route, and work log.
- Important decisions: user routes are `/users/{username}/overview`, `/files`, and `/apps`; direct URLs select the matching user and section; `/users` defaults to the first user's overview.
- Validation: TypeScript syntax parser and `git diff --check`; no frontend production build.
- Known follow-up: User Apps is an empty state until per-user app assignments are implemented.

### 2026-07-17 - List all home-directory users

- Goal: Make `/users` list every directory directly under `/home`.
- Files changed: Linux users service/tests and work log.
- Important decisions: removed the `user-` prefix filter; directories without an `/etc/passwd` account are still listed with UID -1; regular files are ignored.
- Validation: targeted Go tests and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - Configurable automatic Linux usernames

- Goal: Make automatic username generation optional for new Linux users.
- Files changed: settings defaults/validation/UI, user creation UI/backend/tests, and work log.
- Important decisions: `users_auto_username` defaults to false; manual usernames are required and validated when disabled; automatic names use `user-` plus eight lowercase alphanumeric characters.
- Validation: targeted Go tests, TypeScript syntax parser, and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - Nested settings routes

- Goal: Give each Settings section a dedicated URL.
- Files changed: route matcher, main sidebar, Settings route, and work log.
- Important decisions: routes are `/settings/general`, `/settings/users`, and `/settings/apps`; `/settings` falls back to General Settings.
- Validation: TypeScript syntax parser and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - Prefixed settings and Linux user defaults

- Goal: Rename User Settings to Users Settings and persist defaults used by Linux user creation.
- Files changed: settings database/service/API, app context, Settings route, user-add route, tests, and work log.
- Important decisions: sidebar order is General → Users → Apps; keys use `general_`, `users_`, and `apps_`; legacy keys migrate automatically; useradd uses configured shell, home base, and create-home preference.
- Validation: targeted Go tests and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - User settings and sortable app shortcuts

- Goal: Add User Settings and redesign Apps Settings for installed and pinned apps.
- Files changed: Settings route and work log.
- Important decisions: Apps Settings uses installed/header columns; pinned apps support drag/drop and accessible up/down sorting; the narrow header-pin subtitle was removed.
- Validation: `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - App header pin action

- Goal: Add or remove each app from the global header directly from its app page.
- Files changed: Apps route and work log.
- Important decisions: icon-only Pin/PinOff action sits beside service controls and uses the existing SQLite-backed `header_apps` setting.
- Validation: `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - Nested app routes

- Goal: Give every app a dedicated URL such as `/apps/nginx` and `/apps/docker`.
- Files changed: route matcher, main sidebar, Header shortcuts, Apps route, and work log.
- Important decisions: `/apps` remains valid; app selection updates browser history; Back/Forward restores the selected app.
- Validation: `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - SQLite settings and app header shortcuts

- Goal: Persist panel settings and configurable app shortcuts in SQLite.
- Files changed: settings service/routes/tests, route dependencies, app context/API map, Header, Apps route, Settings route, and Go module files.
- Important decisions: default database path is `~/.ppt-server-panel/data/db.sqlite`; `settings` uses key/value rows; Settings sidebar includes General Settings and Apps Settings; header shortcuts are configurable per app.
- Validation: targeted Go tests and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - General settings sidebar

- Goal: Add the first Settings section with persistent app identity and appearance preferences.
- Files changed: app context, dashboard layout, color-mode utilities/switch, Settings route, and app-settings utility.
- Important decisions: Settings uses a left sidebar with `General Settings`; App Name updates the header/document title; color mode supports System, Light, and Dark and applies immediately.
- Validation: `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-17 - Post-update reload countdown

- Goal: Avoid reloading while the reverse proxy may still return a transient 502 after API restart.
- Files changed: `client/src/_layouts/_components/header.tsx`, `.agents/works.md`.
- Important decisions: wait 10 seconds after successful reconnect and update confirmation, show the countdown in the modal, then reload the window.
- Validation: `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-16 - Update reload and consolidated PHP app

- Goal: Reload safely after update reconnect and represent PHP versions as configuration of one app.
- Files changed:
  - `client/src/_layouts/_components/header.tsx`
  - `client/src/routes/apps/index.tsx`
  - `services/apps.go`
  - `services/apps_test.go`
- Important decisions:
  - Window reload occurs only after two successful health responses and update confirmation.
  - PHP is a single Apps sidebar entry; detected PHP 8.1–8.4 versions appear in its configuration panel.
  - PHP service detection is limited to services matching detected versions.
- Validation: targeted Go tests and `git diff --check`; no frontend production build.
- Known follow-up: none.

### 2026-07-16 - Cross-distribution app detection

- Goal: Add Docker, Podman, Node.js, and parallel PHP versions to the Apps panel.
- Files changed:
  - `services/apps.go`
  - `services/apps_test.go`
  - `client/src/routes/apps/index.tsx`
- Important decisions:
  - Added PHP 8.1–8.4 as separate apps.
  - Detection supports Debian/Ubuntu versioned PHP-FPM names, RHEL/Remi paths and units, and Arch/RHEL generic PHP-FPM units.
  - Node.js is installation-only and does not expose system service controls.
  - Docker and Podman detect their native systemd service/socket states.
- Validation: targeted Go tests and `git diff --check` passed; frontend production build was intentionally not run for this incremental UI change.
- Known follow-up: service action buttons still use the existing client-side placeholder behavior and need a backend action endpoint before they control real services.

### 2026-07-08 - Debian RPM Arch installer support

- Goal: Update app/install docs so the service can install runtime dependencies on Debian, RPM-based systems, and Arch.
- Files changed:
  - `README.md`
  - `scripts/install.sh`
  - `.agents/works.md`
- Important decisions:
  - Kept the app's cgo/libcrypt auth implementation because Linux password login needs `crypt(3)`.
  - Added `pacman` support to install `libxcrypt-compat` on Arch Linux.
  - README now documents runtime dependency commands for Debian/Ubuntu, RHEL/Fedora/Amazon Linux, and Arch Linux.
- Validation: pending.
- Known follow-up: Push updated installer and rebuilt dist binaries before testing the remote install command on fresh VPS images.

### 2026-07-08 - libcrypt runtime dependency

- Goal: Fix VPS runtime error `/usr/local/bin/ppt-server-panel: error while loading shared libraries: libcrypt.so.1`.
- Files changed:
  - `README.md`
  - `scripts/install.sh`
  - `.agents/works.md`
- Important decisions:
  - Kept cgo/libcrypt auth path because Linux user login verifies `/etc/shadow` hashes through `crypt(3)`.
  - Added installer check for `libcrypt.so.1`.
  - Installer now attempts to install `libcrypt1` on apt systems and `libxcrypt-compat` on dnf/yum/apk systems before starting service.
  - README now documents the runtime dependency and manual recovery command.
- Validation: pending.
- Known follow-up: Push updated installer to the distribution repo before relying on one-line remote install.

### 2026-07-08 - GitHub raw 429 install workaround

- Goal: Investigate install command failing with `curl: (22) The requested URL returned error: 429`.
- Files changed:
  - `README.md`
  - `scripts/install.sh`
  - `.agents/works.md`
- Important decisions:
  - Confirmed `github.com/.../raw/...` redirects to `raw.githubusercontent.com`, which returned `HTTP/2 429` from GitHub/Fastly.
  - Confirmed jsDelivr URLs returned `HTTP/2 200` for `scripts/install.sh`, `bin/ppt-server-panel`, and `bin/pptctl`.
  - Updated installer default `BIN_URL` and README install commands to use jsDelivr.
- Validation:
  - `curl -I -L https://github.com/antoine-mai/mthan-tools-vps/raw/main/scripts/install.sh`
  - `curl -fsSL https://raw.githubusercontent.com/antoine-mai/mthan-tools-vps/main/scripts/install.sh | head -5`
  - `curl -I -L https://cdn.jsdelivr.net/gh/antoine-mai/mthan-tools-vps@main/scripts/install.sh`
  - `curl -I -L https://cdn.jsdelivr.net/gh/antoine-mai/mthan-tools-vps@main/bin/ppt-server-panel`
  - `curl -I -L https://cdn.jsdelivr.net/gh/antoine-mai/mthan-tools-vps@main/bin/pptctl`
- Known follow-up: Push changes to GitHub so the public install command uses the updated script.

### 2026-07-08 - README and installer help

- Goal: Update installation documentation to match `scripts/install.sh`.
- Files changed:
  - `README.md`
  - `scripts/install.sh`
  - `.agents/works.md`
- Important decisions:
  - Documented root install flow, installed files, root URL, `--reinstall`, environment overrides, and common systemd commands.
  - Added the same environment override details to `install.sh --help`.
- Validation: not run; documentation/help text only.
- Known follow-up: none.

### 2026-07-07 - Agent rules and handoff files

- Created `.agents/rules/project.md` with project rules for future agents.
- Created `.agents/works.md` as the shared handoff log.
- Observed pre-existing modified files:
  - `client/src/_layouts/_components/header.tsx`
  - `client/src/routes/root/users/index.tsx`
- Validation: not run; documentation-only change.

## Handoff Template

### YYYY-MM-DD - Short task title

- Goal:
- Files changed:
- Important decisions:
- Validation:
- Known follow-up:
# Public distribution layout migration

- Goal: Move the published installer to `public/install.sh` and all distribution artifacts from `bin`/`dist` into `public/dist`.
- Changed: build scripts, Makefile, development watcher exclusions, updater URLs, static-client fallbacks, README, and project rules now use the public layout.
- Validation: `bash -n` passed for build, control build, deploy, and installer scripts; `git diff --check` passed. Full builds were not run.
