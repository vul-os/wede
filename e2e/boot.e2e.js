/**
 * boot.e2e.js — THE BOOT GUARD for wede.
 *
 * wede had unit tests (5 files, pure lib/ helpers) and no browser test. `vite
 * build` exiting 0 says nothing about whether the emitted bundle RUNS: two apps
 * in this suite shipped a blank screen behind a green build — one from an
 * unresolved import the bundler turned into a module that throws on load, one
 * from a duplicated React that broke every hook.
 *
 * wede is the most exposed app in this group. Its bundle pulls in CodeMirror
 * (16 language modes), xterm, Yjs + y-websocket, and CodeMirror's LSP client —
 * roughly 1 MB of third-party code, none of it ever loaded in a browser by any
 * test. It is also gated: the app renders THREE different top-level surfaces
 * depending on localStorage (ThemePicker → Login → IDE), so booting "/" only
 * ever proved the first and cheapest of them works. The heavy bundles live
 * behind the *third*.
 *
 * Defects this file catches:
 *   - ANY uncaught exception while loading/rendering the built bundle, at EACH
 *     of the three gated surfaces (not just the first-visit one).
 *   - React mounting nothing (empty #root) → the blank screen.
 *   - The entry chunk being served as HTML instead of JS (a base-path/SPA
 *     fallback bug: HTTP 200, boots nothing, naive checks still pass).
 *   - The IDE — CodeMirror/xterm/Yjs and all — failing to mount for an
 *     authenticated user. This is the surface real users spend all their time
 *     in, and it was the one nothing had ever loaded.
 *   - A dead backend blanking the app instead of degrading.
 */

import { test as bare, expect } from '@playwright/test'
import { installBackend, watchForCrashes, seed } from './fixtures.js'

bare('first visit boots the built bundle and shows the theme picker', async ({ page }) => {
  await installBackend(page)
  await seed(page, 'fresh')
  const { pageErrors, failedRequests } = watchForCrashes(page)

  await page.goto('/')

  const root = page.locator('#root')
  await expect(root).not.toBeEmpty()

  // The right surface — a first visit with no stored theme must ask for one.
  await expect(page.getByRole('heading', { name: /welcome to wede/i })).toBeVisible()
  await expect(page.getByRole('button', { name: /midnight/i })).toBeVisible()
  await expect(page.getByRole('button', { name: /daylight/i })).toBeVisible()

  expect(pageErrors, 'uncaught exception(s) while booting the built bundle').toEqual([])
  expect(failedRequests, 'asset(s) failed to load').toEqual([])
})

bare('entry chunk is served as JavaScript, not an HTML fallback', async ({ page }) => {
  // If the entry chunk ever 200s as text/html (base-path or SPA-fallback bug),
  // the browser refuses to execute it, React never boots, and the page still
  // "loads" — a silent blank screen behind a perfectly healthy HTTP response.
  await installBackend(page)
  await seed(page, 'fresh')

  const chunks = []
  page.on('response', (res) => {
    if (/\/assets\/.*\.js$/.test(new URL(res.url()).pathname)) {
      chunks.push({ url: res.url(), type: res.headers()['content-type'] || '', status: res.status() })
    }
  })

  await page.goto('/')
  await expect(page.locator('#root')).not.toBeEmpty()

  expect(chunks.length, 'index.html referenced no entry script').toBeGreaterThan(0)
  for (const c of chunks) {
    expect(c.status, `${c.url} did not 200`).toBe(200)
    expect(c.type, `${c.url} was not served as JavaScript`).toMatch(/javascript|ecmascript/i)
  }
})

bare('a themed, signed-out visit boots to the login screen', async ({ page }) => {
  // Surface #2 of three. Reached only when a theme is stored — so it is a
  // DIFFERENT top-level render than the one above, with its own crash surface.
  await installBackend(page)
  await seed(page, 'themed')
  const { pageErrors } = watchForCrashes(page)

  await page.goto('/')

  await expect(page.getByRole('heading', { name: 'wede', exact: true })).toBeVisible()
  await expect(page.getByPlaceholder('Enter password')).toBeVisible()
  await expect(page.locator('#root')).not.toBeEmpty()

  expect(pageErrors, 'the login surface threw').toEqual([])
})

bare('an authenticated visit boots the FULL IDE (CodeMirror, xterm, Yjs)', async ({ page }) => {
  // Surface #3 — the one that matters, and the one nothing had ever loaded in a
  // browser. This is where the ~1 MB of CodeMirror/xterm/Yjs actually gets
  // evaluated. An unresolved import or a broken module in ANY of it lands here
  // and nowhere else; `npm run build` and the 5 unit-test files stay green.
  await installBackend(page)
  await seed(page, 'authed')
  const { pageErrors, failedRequests } = watchForCrashes(page)

  await page.goto('/')

  // The IDE really mounted: the file explorer resolved the workspace and
  // rendered the tree the mocked backend served.
  await expect(page.getByText('README.md')).toBeVisible()
  await expect(page.getByText('main.go')).toBeVisible()
  await expect(page.locator('#root')).not.toBeEmpty()

  // And we are past the gates — no theme picker, no login.
  await expect(page.getByPlaceholder('Enter password')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: /welcome to wede/i })).toHaveCount(0)

  expect(pageErrors, 'the IDE threw while mounting the built bundle').toEqual([])
  expect(failedRequests, 'asset(s) failed to load in the IDE').toEqual([])
})

bare('a dead backend degrades instead of blanking the app', async ({ page }) => {
  // Every wede install starts with the Go backend possibly not up yet, and the
  // token in localStorage is trusted optimistically. If a rejection ever escapes
  // (App.fetchWorkspace, useWorkspaces.refresh, useAuth's mount check), React
  // unmounts the tree and the user gets a white page on an HTTP 200.
  await seed(page, 'authed')
  await page.route('**/api/**', (route) => route.abort('connectionrefused'))
  const { pageErrors } = watchForCrashes(page)

  await page.goto('/')

  // It must still render *something* — the app's own loading/degraded state —
  // rather than an empty root.
  await expect(page.locator('#root')).not.toBeEmpty()
  expect(pageErrors, 'a dead backend produced an uncaught exception').toEqual([])
})
