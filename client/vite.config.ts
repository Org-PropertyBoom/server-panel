import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tsconfigPaths from "vite-tsconfig-paths";

// The Go server embeds `client/build` (see main.go `//go:embed all:client/build`),
// so the Vite build output dir stays `build` (not Vite's default `dist`).
// tsconfigPaths() makes Vite honor tsconfig's `baseUrl: "src"` so absolute imports
// like `_utils/api` / `_layouts/dashboard` / `runtime` resolve.
export default defineConfig({
  plugins: [react(), tsconfigPaths()],
  build: {
    outDir: "build",
    emptyOutDir: true,
  },
  server: {
    port: 3000,
    open: false,
  },
  test: {
    globals: true,
    environment: "jsdom",
    setupFiles: "./src/setupTests.ts",
  },
});
