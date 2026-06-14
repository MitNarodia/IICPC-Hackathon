import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
// The dashboard is a static SPA. In dev, Vite proxies API/WS calls to the
// leaderboard-service so the browser talks to a single origin (no CORS dance).
// In production it is served by nginx (see frontend/Dockerfile) and reads the
// API base from the injected window.__TRACK3_API__ or VITE_* env at build time.
export default defineConfig({
    plugins: [react()],
    server: {
        port: 5173,
        proxy: {
            '/v1': {
                target: process.env.VITE_PROXY_TARGET || 'http://localhost:8080',
                changeOrigin: true,
                ws: true,
            },
        },
    },
    build: {
        outDir: 'dist',
        sourcemap: true,
    },
});
