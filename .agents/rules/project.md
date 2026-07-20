# Project Rules

## Scope

- Work in `/home/server/htdocs/ppt/server-panel`.
- Respect existing user changes. Check `git status --short` before editing and do not revert unrelated modified files.
- Keep edits focused on the requested task. Avoid broad refactors unless they are required for correctness.

## Stack

- Backend: Go, entrypoint in `main.go`, routes under `routes/`, services under `services/`.
- Frontend: React 18 + TypeScript under `client/`, using `react-scripts`, Tailwind, Radix UI, and `lucide-react`.
- Go build outputs live in `public/dist/`; the published installer lives at `public/install.sh`. React build output lives in `client/build/`.

## Commands

- Backend tests: `make test`
- Backend formatting: `make fmt`
- Backend build: `make build`
- Frontend build: `cd client && npm run build`
- Frontend tests: `cd client && npm test`

## Backend Guidelines

- Keep route registration in `routes/` and business logic in `services/`.
- Root-only or local/internal POST routes belong under `routes/post/`.
- Preserve the security boundary documented in `README.md`: root-only `/post/*` routes must reject cross-origin public calls and accept only same-host root panel or localhost requests.
- Use Go standard formatting with `gofmt` or `make fmt`.
- Add or update Go tests when changing route behavior, service logic, auth, filesystem operations, Linux users, or root-only helpers.

## Frontend Guidelines

- Follow existing React/TypeScript component patterns in `client/src`.
- Use existing styling conventions and Tailwind utilities before adding new styling systems.
- Use `lucide-react` icons for icon buttons when an appropriate icon exists.
- Keep operational UI dense, clear, and task-focused. This project is a VPS management tool, not a marketing site.
- Avoid layout shifts and text overflow on mobile and desktop.

## Agent Handoff

- Update `.agents/works.md` after meaningful changes.
- Record what changed, files touched, validation run, and any known follow-up.
- If validation cannot be run, record the exact reason.
