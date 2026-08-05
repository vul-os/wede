/**
 * ide.e2e.js — the core wede flows, in a real browser, against the built bundle:
 * sign in, open a file, edit it, save it.
 *
 * The boot guard proves the IDE is not blank. This proves it is not INERT — that
 * the editor is a real editor and that the app's central loop (explorer → open →
 * edit → persist) actually works in the bundle we ship.
 *
 * Defects this file catches:
 *   - Login accepting/rejecting without ever calling the backend, or losing the
 *     server's rejection (the attempts-remaining lockout counter is a security
 *     surface: silently swallowing it makes brute-force feedback disappear).
 *   - A successful login not transitioning to the IDE (the token→render gate in
 *     App.jsx is the whole session mechanism).
 *   - Clicking a file in the explorer not opening it, or opening it EMPTY —
 *     CodeMirror mounting but never receiving the document. In a browser this is
 *     a blank editor pane; in jsdom it is invisible, because CodeMirror needs
 *     real layout to render lines at all. No test in this repo could see it.
 *   - Typing not reaching the CodeMirror document (a broken extension/keymap
 *     wiring — CodeMirror silently swallows input if the state is misconfigured).
 *   - Save not persisting: the write never reaching /api/files/write, or reaching
 *     it with the WRONG CONTENT. That is data loss, and it is the single worst
 *     bug an editor can have.
 */

import { test, expect } from './fixtures.js'
import { test as bare } from '@playwright/test'
import { installBackend, watchForCrashes, seed, PASSWORD } from './fixtures.js'

bare('a wrong password is rejected with the attempts-remaining warning', async ({ page }) => {
  const backend = await installBackend(page)
  await seed(page, 'themed')
  const { pageErrors } = watchForCrashes(page)

  await page.goto('/')
  await page.getByPlaceholder('Enter password').fill('not-the-password')

  const login = page.waitForResponse((r) => r.url().includes('/api/auth/login'))
  await page.locator('button[type="submit"]').click()
  await login

  // The server's rejection must reach the user, with the lockout countdown.
  // (The count is deliberately rendered twice — in the error banner and as a
  // hint under the form — so scope to the banner rather than matching both.)
  await expect(page.getByText('Wrong password. 2 attempts remaining.')).toBeVisible()

  // Still on the login screen — no session was granted.
  await expect(page.getByPlaceholder('Enter password')).toBeVisible()
  expect(backend.loginAttempts).toHaveLength(1)
  expect(pageErrors).toEqual([])
})

bare('signing in transitions to the IDE', async ({ page }) => {
  const backend = await installBackend(page)
  await seed(page, 'themed')
  const { pageErrors } = watchForCrashes(page)

  await page.goto('/')
  await page.getByPlaceholder('Your name (for collaboration)').fill('dev')
  await page.getByPlaceholder('Enter password').fill(PASSWORD)

  const login = page.waitForResponse((r) => r.url().includes('/api/auth/login'))
  await page.locator('button[type="submit"]').click()
  await login

  // The token gate opened and the IDE mounted with the workspace resolved.
  await expect(page.getByText('README.md')).toBeVisible()
  await expect(page.getByPlaceholder('Enter password')).toHaveCount(0)

  // The credentials really went over the wire (not just into local state).
  expect(backend.loginAttempts[0]).toMatchObject({ password: PASSWORD, username: 'dev' })
  expect(pageErrors).toEqual([])
})

test('opens a file from the explorer into a real CodeMirror editor', async ({ wede }) => {
  const { page } = wede
  await seed(page, 'authed')
  await page.goto('/')

  await page.getByText('main.go').click()

  // CodeMirror actually mounted AND received the document. `.cm-line` only
  // exists once CodeMirror has laid the document out — which needs real layout,
  // so jsdom can never assert this.
  const editor = page.locator('.cm-content')
  await expect(editor).toBeVisible()
  await expect(page.locator('.cm-line').first()).toBeVisible()

  // The content is the file the backend served — not an empty buffer, and not
  // some other file's contents.
  await expect(editor).toContainText('package main')
  await expect(editor).toContainText('println("hi")')
})

test('edits a file and saves it back with the correct content', async ({ wede }) => {
  // NOTE ON WHICH SAVE PATH THIS IS. wede has two:
  //   - CRDT collab (Yjs + y-websocket), where the backend persists the doc and
  //     Mod-s is deliberately a NO-OP ("a manual REST save would fight it",
  //     Editor.jsx). This requires editorSettings.collab, which defaults to FALSE.
  //   - Single-user REST save (Mod-s → PUT .../files/write), which is therefore
  //     what EVERY user gets by default.
  // This exercises the default path — the one whose failure means silent data
  // loss for the common case.
  const { page, wrote } = wede
  await seed(page, 'authed')
  await page.goto('/')

  await page.getByText('README.md').click()

  const editor = page.locator('.cm-content')
  await expect(editor).toContainText('Hello from the wede E2E backend.')

  // Type into the real editor. If the keymap/state wiring is broken, CodeMirror
  // silently swallows the input and the document never changes — a bug that
  // looks like "the editor is read-only" to users and like nothing at all to a
  // unit test.
  await editor.click()
  await page.keyboard.press('ControlOrMeta+End')
  await page.keyboard.type('\nEdited by the boot guard.')
  await expect(editor).toContainText('Edited by the boot guard.')

  // Save. The write is a PUT to the WORKSPACE-SCOPED path
  // (/api/workspaces/<id>/files/write) — the frontend rewrites legacy unscoped
  // paths to the active workspace at runtime, and a regression there would send
  // every save to the wrong workspace.
  const write = page.waitForRequest(
    (r) => /\/files\/write$/.test(new URL(r.url()).pathname) && r.method() === 'PUT',
  )
  await page.keyboard.press('ControlOrMeta+s')
  const req = await write
  expect(new URL(req.url()).pathname, 'the save was not scoped to the active workspace')
    .toMatch(/^\/api\/workspaces\/[^/]+\/files\/write$/)

  // THE ASSERTION THAT MATTERS: the bytes that reached the backend are the bytes
  // the user sees. A save that persists stale or empty content is silent data
  // loss — the worst bug an editor can have, and one no unit test here could see.
  await expect.poll(() => wrote.length, { message: 'no write reached the backend' }).toBeGreaterThan(0)
  const body = wrote[wrote.length - 1]
  expect(body.path).toBe('README.md')
  expect(body.content).toContain('Hello from the wede E2E backend.')
  expect(body.content).toContain('Edited by the boot guard.')
})
