# Agent Work Log

This file is for handoff between agents. Keep entries concise, factual, and newest-first.

## Current Project Context

- Repository: `/home/server/htdocs/mthan/vps`
- Product: MThan VPS, a Go API with a React/TypeScript client.
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
- Important decisions: default database path is `~/.mthan-vps/data/db.sqlite`; `settings` uses key/value rows; Settings sidebar includes General Settings and Apps Settings; header shortcuts are configurable per app.
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

- Goal: Fix VPS runtime error `/usr/local/bin/mthan-vps: error while loading shared libraries: libcrypt.so.1`.
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
  - Confirmed jsDelivr URLs returned `HTTP/2 200` for `scripts/install.sh`, `bin/mthan-vps`, and `bin/mthanctl`.
  - Updated installer default `BIN_URL` and README install commands to use jsDelivr.
- Validation:
  - `curl -I -L https://github.com/antoine-mai/mthan-tools-vps/raw/main/scripts/install.sh`
  - `curl -fsSL https://raw.githubusercontent.com/antoine-mai/mthan-tools-vps/main/scripts/install.sh | head -5`
  - `curl -I -L https://cdn.jsdelivr.net/gh/antoine-mai/mthan-tools-vps@main/scripts/install.sh`
  - `curl -I -L https://cdn.jsdelivr.net/gh/antoine-mai/mthan-tools-vps@main/bin/mthan-vps`
  - `curl -I -L https://cdn.jsdelivr.net/gh/antoine-mai/mthan-tools-vps@main/bin/mthanctl`
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
