import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:9090',
        changeOrigin: true,
        ws: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Keep upstream licence banners (containing @license / @preserve, or /*! …)
    // wherever the toolchain surfaces them, instead of Vite's default of
    // stripping all comments while minifying. NOTE: with this Vite 8 + oxc +
    // React 19 toolchain, oxc's minifier still drops banners that originate in
    // unminified dependency source, so this is best-effort only — the AUTHORITATIVE
    // attribution is the generated /licenses.txt (served by the backend), which
    // carries the full licence text of every bundled package including React.
    rollupOptions: { output: { comments: { legal: true } } },
  },
})
