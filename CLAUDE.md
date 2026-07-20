# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Ppt Server Panel ("ppt-server-panel") is a Linux VPS management panel: a single Go binary that embeds a React/TypeScript client. It authenticates Linux users against `/etc/shadow`, manages Linux users/apps/containers/vhosts, and serves an operational dashboard. Deployed via systemd (`ppt-server-panel@root.service`), typically on `:2205`.

Note: source refers to itself as module `ppt/server-panel`; the repo path here is `server-panel` but backend rules/logs reference `/home/server/htdocs/ppt/server-panel` (the deploy checkout).

## Commands

Backend (from repo root; the Makefile targets are the canonical entry points):
- `make run` — run the server (`go run .`). Port comes from `~/.ppt-server-panel/config.yaml`, else `APP_ADDR`, else `:2205`. Override with `APP_ADDR=:8000`.
- `make dev` — `scripts/dev.sh`, a poll-based watcher that rebuilds/restarts on `.go`/`go.mod`/`go.sum` changes (defaults to `:8000`).
- `make test` — `go test . ./routes/... ./services/...`. Run a single test: `go test ./services/ -run TestName -v`.
- `make fmt` — `go fmt ./...`. Use before committing Go changes.
- `make build` — builds both binaries into `public/dist/` (`build-app` + `build-ctl`).
- `make tidy` — `go mod tidy`.

Frontend (from `client/`):
- `npm run build` — production build into `client/build/` (Create React App / react-scripts).
- `npm test` — Jest + React Testing Library.
- `npm start` — CRA dev server on `:3000` (standalone; the Go server serves the built client, not this).

Full release build: `scripts/build.sh` generates `version.json`, builds the client, runs Go tests, builds both binaries (CGO, linux/amd64), and optionally commits/pushes to a separate dist repo. `scripts/deploy.sh` = build (`--no-push`) then `scripts/push.sh`.

## Architecture

### One module, two binaries (build tags)
`main.go` (`//go:build !ctl`) is the server. `main-ctl.go` (`//go:build ctl`) is the `pptctl` CLI, built with `-tags ctl`. Both are `package main` in the same module; the tag selects which `main()` compiles.

### Root vs user runtime mode
At startup `services.NewStartupService()` checks `os.Geteuid()`. Euid 0 → **root mode** (full panel: Linux user management, all `/post/*` routes, container/system control). Non-root → **user mode** (scoped to that user). `StartupConfig.IsRoot` gates behavior throughout, and the mode selects which embedded client subtree is served (`client/build/root` vs `client/build/user`, falling back to `client/build`).

### Three route groups (`routes/`)
`routes.Register` wires three namespaces onto one `http.ServeMux`:
- **`routes/api/`** — public API. Wrapped in `public()` (permissive CORS `*`). Session-cookie auth via `services.SessionCookieName`; most endpoints call `requestSession` and 401 without a valid session.
- **`routes/post/`** — root-only internal/control routes. Every handler is wrapped in `postOnly` = `sameDomainOrLocalhostOnly(rootOnly(...))`: rejects non-root processes (403) **and** rejects cross-origin public calls, accepting only requests whose Origin/Referer host matches the panel host or is localhost. **This is a security boundary — preserve it when adding `/post/*` routes.**
- **`services/router`** — serves the embedded React SPA at `/`, injecting server state into `index.html` as `window.__VPS_RUNTIME__` (see `client/src/runtime.ts`). Falls back to `index.html` for SPA client-side routing.

### Login flow (important indirection)
`POST /api/login` (public) does **not** verify the password itself. It proxies to the internal root process via HTTP `POST /post/user/login` (`postClient` in `routes/api/main.go`, base URL from `router.PostBaseURL`). Only the **root** process can read `/etc/shadow`, so password verification lives behind the root-only `/post/*` boundary; the public API just brokers the credential check and issues the session cookie.

### Auth requires CGO + libcrypt
`services/auth_linux_cgo.go` (`//go:build linux && cgo`) calls C `crypt(3)` (`-lcrypt`) to verify shadow hashes. The non-cgo stub (`auth_nocgo.go`) always returns `ErrAuthUnavailable`. Builds must keep `CGO_ENABLED=1`, and the deployed binary needs `libcrypt.so.1` at runtime (installer handles the distro package). Do not "simplify" auth into pure Go — shadow verification depends on the system crypt.

### Services layer
`services/` holds all business logic; `routes/` should stay thin (registration + request/response wiring). Notable services: `SettingsService` (SQLite at `~/.ppt-server-panel/data/db.sqlite`, key/value `settings` table + `apis` table), `SessionService`, `SystemService`, container/vhost/app/linux-user services. Distro-specific logic branches on `StartupConfig.OSBranch` (`debian`/`rhel`/`arch`/`alpine`) detected from `/etc/os-release`.

### Client (`client/src/`)

CRA + TypeScript (strict, `target: es5`) + Tailwind + shadcn/ui (new-york style, `components.json`) + Radix + `lucide-react` icons + `react-router-dom` v6. `baseUrl: src`, so imports are absolute from `src` (e.g. `import Api from "_utils/api"`, `_layouts/dashboard`). Underscore-prefixed folders (`_components`, `_contexts`, `_layouts`, `_styles`, `_utils`) are shared infrastructure; `routes/` holds pages.

**Server-injected runtime is the source of truth for mode.** `runtime.ts` reads `window.__VPS_RUNTIME__` (injected into `index.html` by the Go router). `runtime.isRoot` / `runtime.mode` drive nearly everything client-side — which routes register, which API base path is used, and feature gating. There is no client-side "am I root" guess; it comes from the server.

**API routing mirrors the backend split (`_utils/api.ts`).** `Api.current` returns the root map (`/post/*`) when `runtime.isRoot`, else the user map (`/api/*`), so page code calls `Api.current.settings` etc. without branching. When adding an endpoint the client consumes, add it to **both** `rootApi` and `userApi` maps.

**Provider tree (`main.tsx`):** `BrowserRouter` → `AppProvider` → `TerminalProvider` → `Routes` + a persistent `GlobalTerminal`. `UserProvider` is nested lower, inside `DashboardLayout`, so auth state re-initializes per page shell.
- `_contexts/app.tsx` (`useApp`) — panel identity/settings: app name, header-pinned apps, color mode, and the SQLite-backed `settings` map. Writes go to `Api.current.settings` (PUT) and update local state optimistically; keys are prefixed (`general_`, `users_`, `apps_`).
- `_contexts/user.tsx` (`useUser`) — login state, persisted in `localStorage.is_logged_in` and validated against `Api.current.session`. **Deliberate resilience: only a 401 logs the user out; network errors and non-401 (5xx/gateway) responses preserve the local session** so an update/restart doesn't bounce the user to `/login`. Keep this behavior when touching session logic.
- `_contexts/terminal.tsx` (`useTerminal`) — multi-tab terminal state lifted above the router so shells survive navigation. Root tab has no `username`; per-user tabs carry one. `Ctrl+\`` toggles the panel (root only).
- `_contexts/apps.tsx` (`useApps`) — localStorage-only app list (client-side, not backed by the API).

**Layout & pages.** Each page renders `DashboardLayout` (`_layouts/dashboard.tsx`) directly — there's no single root shell — which provides the sidebar (collapsed rail + mobile drawer), header, `UserProvider`, redirect-to-`/login` when logged out, and document-title sync. Pages fetch from `Api.current.*` and manage their own state. Routing (`routes/index.tsx`) registers shared routes for all modes and gates `/users/*` plus the catch-all behind `runtime.isRoot` (root → `root/`, user → `user/`); settings use nested `:section` params and `/settings/apps/:app`.

**Terminal transport.** `_components/terminal-panel.tsx` opens a WebSocket to `/post/terminal` (`ws`/`wss` per page protocol) using `xterm` + `xterm-addon-fit`; the shell runs server-side (root, or `su - <username>` for a user tab).

**Theming.** `_utils/color-mode.ts` handles system/light/dark with a `class`-based Tailwind dark mode; preference persists locally and syncs from `general_color_mode` in settings.

## Conventions

- Route registration in `routes/`, logic in `services/`. Root-only/internal routes go under `routes/post/`.
- Add/update Go tests when changing route behavior, service logic, auth, filesystem ops, Linux users, or root-only helpers (test files sit next to sources, e.g. `services/apps_test.go`).
- **All user-facing UI text is English only** (see `AGENTS.md`); no other language unless localization is explicitly requested.
- Keep UI dense and operational — this is a management tool, not a marketing site.
- `.agents/works.md` is a newest-first agent handoff log; append an entry (goal, files changed, decisions, validation) after meaningful changes, per `.agents/rules/project.md`.
- Distribution artifacts live in `public/` (`public/install.sh`, `public/dist/`); the client build output is `client/build/` (git-ignored).
