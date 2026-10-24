import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { loadEnv } from "vite";
import { defineConfig } from "vitest/config";
import { contentSecurityPolicy, cspMeta, spaFallback404 } from "./build/pages.ts";
import { apiOrigin, checkPagesApiOrigin, normalizeBase } from "./src/infrastructure/config.ts";

// The gateway is the single public origin (GATEWAY_URL points the dev server at
// another one). The proxy keeps the browser's Host header (no changeOrigin): the
// gateway's WebSocket accept compares Origin with Host, and both must say
// localhost:3000.
const gateway = { target: process.env.GATEWAY_URL ?? "http://localhost:8080", ws: true };

// Build-time settings (all optional; nginx and `pnpm dev` need none of them):
//   VITE_API_URL  the gateway's origin when it is not the page's own, e.g. https://desk.example.com
//   VITE_BASE     the path the app is served under, e.g. /canton-private-otc-desk/ on GitHub Pages
//   VITE_DEPLOY   "pages": a static deployment — CSP as a meta tag, and a "no desk" page without VITE_API_URL
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, import.meta.dirname, "VITE_");
  // Checked here, so a bad VITE_API_URL fails the build rather than the page.
  const origin = apiOrigin(env.VITE_API_URL);
  checkPagesApiOrigin(env.VITE_DEPLOY, origin);
  return {
    base: normalizeBase(env.VITE_BASE),
    plugins: [
      react(),
      tailwindcss(),
      spaFallback404(),
      ...(env.VITE_DEPLOY === "pages" ? [cspMeta(contentSecurityPolicy({ apiOrigin: origin, meta: true }))] : []),
    ],
    resolve: {
      alias: { "@": path.resolve(import.meta.dirname, "src") },
    },
    server: {
      port: 3000,
      strictPort: true,
      proxy: { "/api": gateway },
    },
    preview: {
      port: 3000,
      strictPort: true,
      proxy: { "/api": gateway },
    },
    build: {
      rolldownOptions: {
        output: {
          // React rarely changes and the other libraries seldom: separate chunks cache longer.
          codeSplitting: {
            groups: [
              { name: "react", test: /node_modules[\\/](react|react-dom|react-router|scheduler)[\\/]/ },
              { name: "vendor", test: /node_modules[\\/]/ },
            ],
          },
        },
      },
    },
    test: {
      include: ["src/**/*.test.ts", "build/**/*.test.ts"],
      environment: "node",
    },
  };
});
