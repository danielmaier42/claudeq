import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import {fileURLToPath, URL} from 'node:url'
import {defineConfig} from 'vite'

// The build lands where the Go daemon embeds it (internal/api/web). `npm run dev`
// proxies /api to a running daemon, so the UI can be worked on with hot reload
// against real data.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {alias: {'@': fileURLToPath(new URL('./src', import.meta.url))}},
  build: {
    outDir: '../internal/api/web',
    emptyOutDir: true,
    chunkSizeWarningLimit: 1200,
  },
  server: {
    proxy: {'/api': process.env.CLAUDEQ_URL ?? 'http://127.0.0.1:10765'},
  },
})
