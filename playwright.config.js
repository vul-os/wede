/**
 * Playwright E2E config — wede.
 *
 * WHY THIS EXISTS: `vite build` exiting 0 proves nothing about whether the
 * bundle it emitted actually RUNS. Two apps in this suite shipped a blank screen
 * behind a green build (an unresolved import that became a module which throws
 * on load; a duplicated React that broke every hook). wede had unit tests for
 * five pure lib/ helpers and NO browser test — its ~1 MB of CodeMirror, xterm
 * and Yjs had never been loaded in a browser by anything.
 *
 * The suite drives the PRODUCTION build via `vite preview` — never the dev
 * server — so chromium executes exactly what `npm run build:all` embeds into the
 * Go binary. The backend is mocked in-browser with page.route + routeWebSocket
 * (see e2e/fixtures.js), so the run is hermetic: no `go run ./cmd/wede`, no real
 * filesystem, no git repo.
 *
 * NOTE: vite.config.js proxies /api to :9090 for the dev server. That proxy does
 * NOT apply here — page.route intercepts inside the browser, before the request
 * ever leaves it, so the suite is unaffected by whether a wede backend is up.
 *
 * Prereqs:  npm run build             (`pretest:e2e` does it)
 *           npx playwright install chromium
 * Run:      npm run test:e2e
 */

import { defineConfig, devices } from '@playwright/test'

// Uncommon port so a stale preview of another Vulos app on 5173/4173 can never
// be mistaken for wede. Override with E2E_PORT.
const PORT = Number(process.env.E2E_PORT ?? 47351)
const BASE_URL = `http://localhost:${PORT}`

export default defineConfig({
  testDir: './e2e',
  testMatch: '**/*.e2e.js',
  // The IDE mounts a lot of third-party code (CodeMirror + xterm + Yjs); a cold
  // first navigation on a loaded machine is not instant.
  timeout: 45_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI ? [['github'], ['list']] : [['list']],
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    serviceWorkers: 'block',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: `npx vite preview --port ${PORT} --strictPort`,
    url: BASE_URL,
    // Never reuse a server on this port — it could be another app, and we would
    // silently test the wrong bundle.
    reuseExistingServer: false,
    timeout: 60_000,
  },
})
