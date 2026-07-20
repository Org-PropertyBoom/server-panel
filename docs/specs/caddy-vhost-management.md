# Safe Caddy vhost management in server-panel (design reference)

> **Status:** design only — no code written yet. Coding is intentionally halted; this
> document captures the full understanding so implementation can proceed later without
> re-deriving it. Approved plan: `~/.claude/plans/federated-cooking-curry.md`.

## 1. Purpose & direction

server-panel is being consolidated as the single server control panel (deployed at
`cp.propertyweb.co`, `:2205`). It will **absorb the Caddy-vhost reconcile + safe-reload
engine** so that managing public web hosts happens inside this panel.

**Decision (Owner, final):** *full replace* — port the engine from the CaddyDash repo into
server-panel and retire CaddyDash. CaddyDash is never deployed; its source
(`../CaddyDash`, a sibling repo) is the reference to port **from**. An earlier "complement"
(server-panel calls a running CaddyDash) decision was reversed.

Today server-panel's `/vhosts` is a **read-only viewer**: `services/vhosts.go` `List()` shells
`caddy adapt --config /etc/caddy/Caddyfile` and parses the JSON; `routes/{api,post}/vhost`
expose status/list/get only. There is no write path, no reload, and no connection to the
desired-state database. This design turns that view into safe **management**.

**Why server-panel is the right host:** its *root mode* already runs as root and already
shells `caddy adapt` as root. Reading the root-only vhosts folder and reloading Caddy as root
is exactly the privilege the safety model requires — see §3.

## 2. The model in one line

Desired state lives in a shared MySQL database. A **reconcile** renders desired rows into
`<host>.caddy` files in the Caddy vhosts folder, **validates** the whole config, then
**reloads** Caddy through its admin API. Files are never edited ad-hoc; Caddy is never
reloaded without validating first.

```
DB rows (desired)  ──render──▶  /home/server/.caddy/<host>.caddy  ──adapt(validate)──▶  JSON  ──POST /load──▶  live Caddy
   (author here)                        (rendered artifact)              (abort gate)                 (as root)
```

## 3. ⚠ Non-negotiable safety contract

This exists because getting it wrong caused a **full production outage on 2026-07-11**: a
reload run as the `caddy` service user read the root-only `/home/server/.caddy` folder as
empty (an `import` glob that matched nothing), so Caddy loaded only its static block and
**dropped every tenant vhost**.

1. **Reload only via the admin API, as root.** Adapt the config to JSON *as root* (root can
   read the folder), then `POST` that **JSON** to `http://localhost:2019/load`. Back up the
   prior live config first via `GET /config/`.
2. **NEVER `systemctl reload caddy`** (runs as the `caddy` user → empty folder read → outage).
3. **NEVER POST a raw Caddyfile** to `/load` (the admin process would re-adapt it as the
   `caddy` user → same empty read).
4. **Validate before every reload.** Adapt the full main Caddyfile; abort on any error **or**
   an empty/suspiciously-short result (`minAdaptedLen`). On abort, files may be on disk but
   Caddy stays untouched.
5. **Dashboard-present assertion.** Refuse to reload any adapted config that does not contain
   the protected dashboard domain — that absence is the outage signature.
6. **First-pass removal suppression.** On the first reconcile of a process, delete nothing
   (write only); removals apply from the second pass. Prevents a cold start from dropping
   live orphan files.
7. **Never auto-prune orphans.** Files with no backing DB row are *reported*, never deleted
   automatically. Removals come only from rows observed inactive/soft-deleted.
8. **Never remove protected or `wildcard_*` files.**
9. **Unknown `server_stack` → skip** (report it); never guess an upstream port
   (cross-stack-misroute risk).
10. **Missing table (MySQL 1146) is tolerated** (read as zero rows, noted, continue). Any
    other DB error is **fatal** — a transient fault must never look like "empty → delete all".
11. **Serialized:** one reconcile at a time (mutex).
12. **Truthful result:** always return the real outcome (reloaded / written / removed /
    removes_suppressed / orphans / skips / error); never fake success.
13. **Protected domains** = the dashboard domain (`app.propertyboom.co`) **and server-panel's
    own `cp.propertyweb.co`** (never lock the panel out of its own front door).
14. **Live-activation gate:** building inert/env-gated code is safe to do anytime; switching
    on the live `/load` reload needs the Owner's **direct, per-activation go-ahead** — a
    relayed/second-hand approval does not clear it.

## 4. Desired state — shared `propertyteam` MySQL (read)

The connection is a named **Data Source** (see §7a), selected by the vhost feature as
its "host-source". Read **all** rows including inactive/soft-deleted (a row flipping
inactive/soft-deleted is how the engine learns a previously-rendered file should now be
removed). A row is **desired** (rendered to a file) iff `is_active=1 AND deleted_at IS NULL`.

| Table | Columns read | Role | Writes |
|---|---|---|---|
| `website_hosts` | `id, host, server_stack, is_active, deleted_at` | tenant sites | **read-only** (stack-owned) |
| `platform_hosts` | `id, host, server_stack, target, is_active, deleted_at` | system/dashboard-app domains | create/update/toggle/soft-delete |
| `platform_redirect_hosts` | `id, host, target, redirect_code, is_active, deleted_at` | edge redirects (global) | create/update/toggle/soft-delete |

server-panel **manages the two platform tables** (its management UI); `website_hosts` stays
owned by the stack apps and is never written here. Writes are soft-delete only (set
`deleted_at`), never hard-delete.

## 5. Render — byte-exact `.caddy` files

Filename: `<host>.caddy`; a wildcard `*.x` maps to `wildcard_x.caddy` (`*` is not a legal
filename char). Host is lower-cased and trimmed. Snippets are **byte-identical** to what the
stack apps write today, so switching the writer produces no folder diff (4-space indent,
trailing newline):

- **Tenant** (`website_hosts`): `reverse_proxy` to the upstream resolved from the
  `server_stack` → `host:port` map. Unknown stack → skip.
  ```
  <host> {
      reverse_proxy <upstream>
  }
  ```
- **System** (`platform_hosts`): `reverse_proxy` to the row's `target`. Empty target → skip.
- **Redirect** (`platform_redirect_hosts`): `redir <target> <code>` (`code <= 0` → 301).
  ```
  <host> {
      redir <target> <code>
  }
  ```

**Stack → upstream port map** (from `design-templates/docs/stack-deploy-ports.md`; config,
never a code literal): `phalcon 127.0.0.1:8002 · laravel 127.0.0.1:8004 · golang
127.0.0.1:8005 · rust 127.0.0.1:8000`. A hardcoded/stale port is dangerous — e.g. a stale
`8000` would route to *rust's* live backend (silent cross-stack misroute), so the port is
always resolved from config, and an unknown stack is skipped, never guessed.

(Optional, tenant-only: an `encode` directive and a `header { … }` security-header block —
never applied to system/admin hosts, which need different framing/CSP.)

## 6. Engine architecture (six packages, pure/IO split)

Ported near-verbatim from `CaddyDash/internal/*` (rewrite import paths
`github.com/Org-PropertyBoom/caddydash/internal/*` → `ppt/server-panel/services/caddy/*`). The
`_test.go` files port too — the pure planning tests are the safety proof.

| Package | From | Responsibility | Deps |
|---|---|---|---|
| `render` | `internal/render` | host → filename + byte-exact snippet (pure) | stdlib |
| `config` | `internal/config` (+`security.go`) | env-driven config, protected hosts, stack-port map | stdlib |
| `vhostfs` | `internal/vhostfs` | atomic temp-file+rename writes, safe `*.caddy` listing/remove; rejects non-basenames, symlinks, traversal | stdlib |
| `db` | `internal/db` (`db.go`+`write.go`+`validate.go`) | MySQL read snapshot of 3 tables; platform-table writes; pure input validation | `go-sql-driver/mysql` |
| `caddyctl` | `internal/caddyctl` | adapt Caddyfile → JSON + reload via admin `/load` + backup `GET /config/` | see §8 |
| `reconcile` | `internal/reconcile` (`plan.go`+`engine.go`) | **the heart**: pure diff (`plan.go`) + serialized apply (`engine.go`) | the above |

**`reconcile/plan.go` (pure)** — given a DB snapshot, the current folder filenames, and
config, computes a `Plan{Writes, Removes, Orphans, Skips}` with no I/O (exhaustively
unit-testable). Encodes the removal/orphan/skip/collision rules from §3.

**`reconcile/engine.go` (I/O, serialized by a mutex)** — one `Reconcile(snapshot)` pass:
1. ensure folder exists + list `*.caddy` names
2. `BuildPlan(...)`
3. **writes** — atomic temp+rename, idempotent (report only changed files)
4. **removes** — *suppressed on the first pass* (report `removes_suppressed`), applied after
5. **validate** — read the main Caddyfile, adapt in-process; **abort before reload** on error
6. **assert** the adapted JSON contains the dashboard domain, else refuse
7. **backup** the prior live config to a timestamped file (best-effort)
8. **reload** — `POST` adapted JSON to admin `/load`
9. return the truthful `Result`

Plus `ReloadOnly()` (re-validate+reload the current folder, mutate nothing — the "force
reload" button) and `DryRun(snapshot)` (read-only drift view for the UI: `would_write`,
`would_remove`, `orphans`, `skips`, `in_sync`, plus each folder file's contents), and
`RemoveFile(name)` (explicit orphan prune — refuses protected/wildcard, caller reconciles
after).

## 7a. Data Sources (implemented — the DB entry point)

The desired-state DB is reached through a generic **Data Sources** feature (built ahead
of the engine): a root-owned list of named, engine-agnostic connections
`{ Name, Engine, Host, Port, Database, User, Password }`, managed from a "Data Sources"
Settings section. An adapter registry (`services/datasources_adapters.go`) maps each
engine to a `database/sql` driver + DSN + `SELECT 1` probe — MySQL and SQLite compiled
in, PostgreSQL registered but its driver not yet imported (adding it later = one import).

- Store: `/etc/ppt-server-panel/datasources.json` (root 0640, atomic writes). Passwords live only
  server-side; the client sees `passwordSet`, and a blank password on save keeps the
  stored one.
- Routes (root-only via `postOnly`): `GET/PUT/DELETE /post/datasources`,
  `POST /post/datasources/test` (per-source ping + probe).
- **The vhost engine picks a Data Source by name** as its host-source and reads the three
  host tables through it. The vhost-specific "count the 3 tables" check belongs to the
  vhost feature's own verify — the generic Data Source Test is engine-agnostic liveness only.

## 7. Mapping into server-panel

- **Packages:** new subtree `services/caddy/{render,config,vhostfs,db,caddyctl,reconcile}`.
- **Root-only engine singleton:** construct once in `main.go` (after `settings`), gated on
  `startup.IsRoot` (nil/skip in user mode); it holds the MySQL handle, the mutex, and the
  `firstDone` flag. Add an `Engine` field to `routes.Dependencies` + `post.Dependencies` and
  forward it through `routes.Register`. Because it's a stateful singleton, pass the instance
  (not a `New…()` per request, unlike today's `services.NewVHostService()`).
- **Routes** under `/post/vhost/*` — already root-only + same-origin/localhost via the
  existing `postOnly` (`rootOnly` + `sameDomainOrLocalhostOnly`) middleware, so no new auth
  is needed for the panel surface:
  - `GET /post/vhost/state` → `Engine.DryRun` (drift view) — **Phase 1, read-only.**
  - `POST /post/vhost/reconcile`, `POST /post/vhost/reload` — **Phase 2.**
  - CRUD for `platform_hosts` + `platform_redirect_hosts`; `POST …/orphans/{adopt,prune}` —
    **Phase 2.**
- **Config/env** (separate from the SQLite panel settings; mirrors `settings.go`'s env
  style): `vhosts_dir=/home/server/.caddy`, `main_caddyfile=/etc/caddy/Caddyfile`,
  `caddy_admin_url=http://localhost:2019`, stack-port map, `protected_domains=[<dashboard>,
  cp.propertyweb.co]`, `backup_dir`, and the `propertyteam` MySQL DSN. Live reload is behind
  env gates, **default off**.
- **Viewer source-of-truth fix:** today `/vhosts` reflects only the adapted
  `/etc/caddy/Caddyfile`. Point it at the **DB desired-state + the folder** so the view
  reflects what the engine actually controls (`/home/server/.caddy/*.caddy`, imported by the
  main file), including in-sync/drift status.
- **UI (`client/src/routes/vhosts/index.tsx`):** Phase 1 adds an in-sync/drift badge (reuse
  the existing TLS-pill pattern). Phase 2 adds add/edit/remove forms (website_hosts
  read-only), a Reconcile button → dry-run **diff** → confirm → apply → truthful result, and
  an orphan adopt/prune panel. No shadcn Dialog exists — reuse the hand-rolled modal/form
  patterns in `client/src/routes/root/users/index.tsx`. The design-templates hub (Stack
  Server Architect) will supply the polished dark-admin screen designs.

## 8. Open decision — adapt strategy

Adapting the Caddyfile to JSON *as root* is the safety-critical step. Two ways:

- **In-process** (`caddyserver/caddy/v2` library) — **the Architect's ruling; the current
  direction.** Byte-for-byte fidelity to the reference engine, typed adapter warnings,
  version-independent from the host `caddy` binary. **Cost:** pulls ~150 transitive deps into
  server-panel (tens of MB, slower CGO cross-compile + `dev.sh` reload loop) and likely a Go
  directive bump to 1.26; also needs an xcaddy-style rebuild to match any non-standard host
  Caddy plugins, or adapt fails.
- **Shell `caddy adapt` as root** — server-panel already does this in `services/vhosts.go`
  via its `commandRunner`. Keeps server-panel a lean single binary (no `caddy/v2`), automatic
  module-parity with the host's real Caddy. Preserves every §3 guard. Deviates from the
  reference implementation but keeps its safety properties. (Only the `caddyctl` package
  differs between the two; `reload.go` — plain `net/http` to `/load` and `/config/` — is
  identical either way.)

## 9. Phasing

1. **Phase 1 — port the engine, INERT.** All six packages + ported tests; root-only engine
   wired; `GET /post/vhost/state` + drift badge. No live reload (env-gated off). Zero
   mutation risk.
2. **Phase 2 — management + reconcile, live reload still gated.** platform-table CRUD,
   reconcile/reload endpoints, dry-run-diff UI, orphan triage. First-pass suppression +
   backup always on; `/load` stays inert until the Owner activates it.
3. **Phase 3 — stack cutover + retire CaddyDash (needs the stack agents).** A separate
   bearer-token `POST /reconcile` for pc/la/go over the Docker bridge; coordinated repoint
   from `CaddyDash:8090` → server-panel; retire CaddyDash; remove the stacks' old admin-API
   fallback only after the folder path is verified live per stack.

## 10. References

- Reference engine (read firsthand): `../CaddyDash/internal/{config,db,render,vhostfs,caddyctl,reconcile}` (+ `_test.go`).
- Safety model / deploy: `../CaddyDash/docs/specs/caddy-vhosts-folder.md`, `../CaddyDash/docs/install.md`.
- server-panel integration points: `main.go`, `routes/main.go`, `routes/post/main.go`,
  `routes/post/vhost/main.go`, `services/vhosts.go`, `client/src/routes/vhosts/index.tsx`.
