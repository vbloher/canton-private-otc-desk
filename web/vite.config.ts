import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The gateway is the single public origin (GATEWAY_URL points the dev server at
// another one). The proxy keeps the browser's Host header (no changeOrigin): the
// gateway's WebSocket accept compares Origin with Host, and both must say
// localhost:3000.
const gateway = { target: process.env.GATEWAY_URL ?? "http://localhost:8080", ws: true };

export default defineConfig({
  plugins: [react(), tailwindcss()],
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
    include: ["src/**/*.test.ts"],
    environment: "node",
  },
});
