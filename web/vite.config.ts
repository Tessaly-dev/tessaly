// SPDX-License-Identifier: Apache-2.0
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
  server: {
    host: "127.0.0.1",
    // changeOrigin sends Host: 127.0.0.1:7777, which the daemon's Host check
    // accepts. The browser still marks requests same-origin, so the CSRF
    // check passes too.
    proxy: {
      "/healthz": { target: "http://127.0.0.1:7777", changeOrigin: true },
      "/api": { target: "http://127.0.0.1:7777", changeOrigin: true },
    },
  },
  test: {
    environment: "jsdom",
  },
});
