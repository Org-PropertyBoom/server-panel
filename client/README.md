# Ppt Server Panel — client

React + TypeScript single-page app, built with [Vite](https://vite.dev/).

## Scripts

- `npm run dev` — start the Vite dev server (http://localhost:3000). Note: API calls hit
  `/post/*` and `/api/*`, which are served by the Go backend — run the panel binary (or
  point a proxy) for a full dev experience.
- `npm run build` — production build into `build/` (the Go server embeds `client/build`
  via `//go:embed`, so the output dir is `build`, not Vite's default `dist`).
- `npm run preview` — serve the production build locally.
- `npm test` — run tests with [Vitest](https://vitest.dev/).

## Notes

- **Absolute imports** (`_utils/...`, `_layouts/...`, `runtime`, …) resolve from `src` via
  tsconfig `baseUrl`, honored in Vite by `vite-tsconfig-paths` (see `vite.config.ts`).
- **Server runtime**: the Go server injects `window.__VPS_RUNTIME__` into the served
  `index.html` (root/user mode, OS, etc.); see `src/runtime.ts`.
- Styling: Tailwind CSS + shadcn/ui; config in `tailwind.config.js` / `postcss.config.js`.
