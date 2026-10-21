import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Override with OB_BACKEND=http://host:port when the API runs elsewhere.
const BACKEND = process.env["OB_BACKEND"] ?? "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  build: { outDir: "dist", emptyOutDir: true, sourcemap: false },
  server: {
    port: 5173,
    proxy: {
      "/api": { target: BACKEND, changeOrigin: false },
      "/mcp": { target: BACKEND, changeOrigin: false },
      "/health": { target: BACKEND, changeOrigin: false },
    },
  },
  test: { environment: "happy-dom", include: ["src/**/*.test.ts"] },
});
