/// <reference types="vitest/config" />
// Config for the chat render benchmarks. Kept out of the default test run
// (vite.config.ts) so `make check` stays fast and deterministic.
//
//   npx vitest run --config vitest.perf.config.ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
    include: ['src/perf/**/*.bench.tsx'],
    testTimeout: 600_000,
  },
  plugins: [react()],
})
