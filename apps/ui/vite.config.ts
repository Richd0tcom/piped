import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tsconfigPaths from "vite-tsconfig-paths";
import tailwindcss from "@tailwindcss/vite";
import { TanStackRouterVite } from "@tanstack/router-plugin/vite";

export default defineConfig({
  plugins: [
    TanStackRouterVite(),
    react(),
    tsconfigPaths(),
    tailwindcss(),
   
  ],
  preview: {
    port: 3000,
    host: '0.0.0.0',
    allowedHosts: ['ui', 'localhost', '127.0.0.1'],
  },
  server: {
    allowedHosts: ['ui', 'localhost', '127.0.0.1'],
  },
  build: {
    outDir: "dist",
    sourcemap: false,
  },
});
