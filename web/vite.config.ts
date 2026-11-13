import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// In development the API runs on :8080; Vite proxies /api so the browser
// sees a single origin, exactly like production where Go serves the build.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": process.env.PULSEWATCH_API ?? "http://localhost:8080",
      "/healthz": process.env.PULSEWATCH_API ?? "http://localhost:8080",
    },
  },
  build: { sourcemap: false, chunkSizeWarningLimit: 700 },
  test: { environment: "jsdom", globals: false },
});
