/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import type { Plugin } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'

// KaTeX's stylesheet lists every font as woff2, woff and ttf, so Vite emits all
// three (~60 files). Every browser we target loads woff2, so strip the legacy
// fallbacks from the @font-face `src` lists before Vite resolves their URLs.
function katexWoff2Only(): Plugin {
  return {
    name: 'katex-woff2-only',
    enforce: 'pre',
    transform(code, id) {
      if (!/[\\/]katex[\\/]dist[\\/]katex(\.min)?\.css$/.test(id)) return null
      return {
        code: code.replace(/,\s*url\([^)]*\.(?:woff|ttf)\)\s*format\("(?:woff|truetype)"\)/g, ''),
        map: null,
      }
    },
  }
}

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
    exclude: ['e2e/**', 'node_modules/**'],
  },
  plugins: [
    katexWoff2Only(),
    react(),
    tailwindcss(),
    VitePWA({
      registerType: 'prompt',
      strategies: 'injectManifest',
      srcDir: 'src/sw',
      filename: 'sw.ts',
      includeAssets: ['favicon.ico', 'favicon-16x16.png', 'favicon-32x32.png', 'apple-touch-icon.png', 'android-chrome-192x192.png', 'android-chrome-512x512.png', 'maskable-icon-512x512.png'],
      manifest: {
        name: 'Botka',
        short_name: 'Botka',
        description: 'AI Task Scheduler & Chat',
        start_url: '/',
        theme_color: '#18181b',
        background_color: '#fafafa',
        display: 'standalone',
        orientation: 'any',
        icons: [
          { src: '/android-chrome-192x192.png', sizes: '192x192', type: 'image/png' },
          { src: '/android-chrome-512x512.png', sizes: '512x512', type: 'image/png' },
          { src: '/maskable-icon-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
        ],
      },
      injectManifest: {
        globPatterns: ['**/*.{js,css,html,ico,png,svg,woff2}'],
      },
    }),
  ],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:5110',
      '/mcp': 'http://localhost:5110',
    },
  },
})
