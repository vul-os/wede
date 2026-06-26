/**
 * wede screenshot generator
 *
 * Captures docs/screenshots/*.png at 1440×900 using Playwright (Chromium).
 *
 * Usage:
 *   npx playwright install chromium   # one-time
 *   npm run screenshots
 *
 * Environment variables:
 *   BASE_URL       wede instance URL  (default: http://localhost:9090)
 *   WEDE_PASSWORD  login password     (default: admin)
 *
 * The screenshotter starts wede pointed at scripts/demo-workspace/ so all
 * captures show a realistic developer project (taskboard — Go API + React).
 * The workspace has 2 commits of git history and an unstaged diff in
 * api/middleware.go so the git panel, graph, and diff view are all populated.
 */

import { chromium } from 'playwright';
import { mkdirSync, writeFileSync, existsSync, appendFileSync, rmSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';
import { spawn, spawnSync } from 'child_process';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');
// WEDE_THEME=dark renders the dark (Midnight) theme into docs/screenshots/dark/.
const THEME = process.env.WEDE_THEME === 'dark' ? 'dark' : 'light';
const OUT_DIR = THEME === 'dark'
  ? resolve(ROOT, 'docs', 'screenshots', 'dark')
  : resolve(ROOT, 'docs', 'screenshots');
const DEMO_WORKSPACE = resolve(__dirname, 'demo-workspace');
const WEDE_BIN = resolve(ROOT, 'wede');

const BASE_URL = process.env.BASE_URL || 'http://localhost:9090';
const PASSWORD = process.env.WEDE_PASSWORD || 'admin';
const VIEWPORT = { width: 1440, height: 900 };

mkdirSync(OUT_DIR, { recursive: true });

// ── demo workspace git setup ──────────────────────────────────────────────────
// scripts/demo-workspace/ is committed as plain files (no nested .git).
// Before running wede we initialise a throwaway git repo there so the git
// panel, diff view, and commit graph are populated with real content.

function git(args, env = {}) {
  return spawnSync('git', ['-C', DEMO_WORKSPACE, ...args], {
    stdio: 'pipe',
    env: { ...process.env, ...env },
  });
}

// commitAs makes a commit with a specific author + date so the demo history looks
// like a real multi-person project.
function commitAs(date, name, email, msg) {
  git(['commit', '-q', '-m', msg], {
    GIT_AUTHOR_NAME: name, GIT_AUTHOR_EMAIL: email,
    GIT_COMMITTER_NAME: name, GIT_COMMITTER_EMAIL: email,
    GIT_AUTHOR_DATE: date, GIT_COMMITTER_DATE: date,
  });
}

// setupDemoWorkspaceGit rebuilds a throwaway git history in scripts/demo-workspace
// with a feature branch + merge, so the commit graph shows real branching lanes.
// The nested .git is auto-ignored by the outer wede repo (git never tracks .git).
function setupDemoWorkspaceGit() {
  rmSync(resolve(DEMO_WORKSPACE, '.git'), { recursive: true, force: true });
  rmSync(resolve(DEMO_WORKSPACE, '.wede'), { recursive: true, force: true }); // fresh chat history
  console.log('  Building demo-workspace history (feat/web branch + merge)...');

  git(['init', '-q', '-b', 'main']);

  // main — backend
  git(['add', 'go.mod', 'package.json', 'README.md', 'api/main.go']);
  commitAs('2026-06-10T09:12:00', 'Ava Chen', 'ava@vulos.org', 'chore: scaffold taskboard (Go API + React UI)');
  git(['add', 'api/handlers.go']);
  commitAs('2026-06-12T14:05:00', 'Imran Paruk', 'imran@vulos.org', 'feat(api): task CRUD handlers (list/create/update/delete)');
  git(['add', 'api/middleware.go']);
  commitAs('2026-06-13T10:22:00', 'Sam Rivera', 'sam@vulos.org', 'feat(api): CORS + auth middleware');

  // feat/web — frontend, branched off main
  git(['checkout', '-q', '-b', 'feat/web']);
  git(['add', 'src/App.jsx']);
  commitAs('2026-06-14T16:18:00', 'Ava Chen', 'ava@vulos.org', 'feat(web): app shell + board layout');
  git(['add', 'src/components/TaskList.jsx', 'src/components/TaskForm.jsx']);
  commitAs('2026-06-15T12:03:00', 'Ava Chen', 'ava@vulos.org', 'feat(web): task list + create form');
  git(['add', 'src/utils/api.js']);
  commitAs('2026-06-16T09:47:00', 'Ava Chen', 'ava@vulos.org', 'feat(web): fetch-based API client');

  // main advances, then merges feat/web (--no-ff keeps the merge commit visible)
  git(['checkout', '-q', 'main']);
  git(['add', 'tests/handlers_test.go']);
  commitAs('2026-06-16T15:30:00', 'Sam Rivera', 'sam@vulos.org', 'test(api): handler unit tests');
  git(['merge', '--no-ff', 'feat/web', '-q', '-m', "Merge branch 'feat/web': task board UI"], {
    GIT_AUTHOR_NAME: 'Imran Paruk', GIT_AUTHOR_EMAIL: 'imran@vulos.org',
    GIT_COMMITTER_NAME: 'Imran Paruk', GIT_COMMITTER_EMAIL: 'imran@vulos.org',
    GIT_AUTHOR_DATE: '2026-06-17T10:00:00', GIT_COMMITTER_DATE: '2026-06-17T10:00:00',
  });

  // Unstaged change so the diff view is populated.
  appendFileSync(
    resolve(DEMO_WORKSPACE, 'api/middleware.go'),
    '\n// rateLimiter stub — TODO: implement with golang.org/x/time/rate\nfunc rateLimiter(next http.HandlerFunc, _ int) http.HandlerFunc {\n\treturn next\n}\n',
  );

  // Seed saved API requests + an environment so the API-client demo is populated.
  const reqDir = resolve(DEMO_WORKSPACE, '.wede', 'requests', 'Tasks');
  mkdirSync(reqDir, { recursive: true });
  writeFileSync(resolve(reqDir, 'list-tasks.json'), JSON.stringify({
    name: 'list-tasks', method: 'GET', url: '{{base}}/api/tasks',
    headers: [{ key: 'Accept', value: 'application/json', enabled: true }],
    params: [], auth: { type: 'none' }, body: { type: 'none' },
  }, null, 2));
  writeFileSync(resolve(reqDir, 'create-task.json'), JSON.stringify({
    name: 'create-task', method: 'POST', url: '{{base}}/api/tasks',
    headers: [], params: [], auth: { type: 'bearer', token: '{{token}}' },
    body: { type: 'json', content: '{\n  "title": "Ship wede",\n  "priority": "high"\n}' },
  }, null, 2));
  const envDir = resolve(DEMO_WORKSPACE, '.wede', 'environments');
  mkdirSync(envDir, { recursive: true });
  writeFileSync(resolve(envDir, 'Local.json'), JSON.stringify({
    name: 'Local', variables: { base: 'http://localhost:8080', token: 'dev-token' },
  }, null, 2));

  console.log('  demo-workspace ready (8 commits, feat/web merged, seeded API requests)');
}

// ── helpers ───────────────────────────────────────────────────────────────────

async function shot(page, name) {
  const file = resolve(OUT_DIR, `${name}.png`);
  await page.screenshot({ path: file, fullPage: false });
  console.log(`  ✓  ${name}.png`);
}

function sleep(ms) {
  return new Promise(r => setTimeout(r, ms));
}

async function waitForIDE(page) {
  await page.waitForFunction(() =>
    document.body.innerText.includes('taskboard') ||
    document.querySelector('.cm-editor') ||
    document.body.innerText.length > 200,
    { timeout: 15000 }
  ).catch(() => {});
  await sleep(900);
}

/**
 * Start the wede binary pointed at the demo workspace.
 * Returns a cleanup function that kills the process.
 * If wede is already reachable, this is a no-op.
 */
async function maybeStartWede() {
  // Check if already running
  try {
    const res = await fetch(`${BASE_URL}/api/auth/check`);
    if (res.ok || res.status === 401) {
      console.log('  wede already reachable — skipping auto-start');
      return () => {};
    }
  } catch (_) {}

  if (!existsSync(WEDE_BIN)) {
    console.log(`  wede binary not found at ${WEDE_BIN} — skipping auto-start`);
    return () => {};
  }

  console.log(`  Starting wede → ${DEMO_WORKSPACE} ...`);
  const proc = spawn(WEDE_BIN, [DEMO_WORKSPACE], {
    stdio: ['ignore', 'pipe', 'pipe'],
    env: { ...process.env, HOME: process.env.HOME },
    cwd: DEMO_WORKSPACE,
  });

  proc.stdout.on('data', d => process.stdout.write(`  [wede] ${d}`));
  proc.stderr.on('data', d => process.stderr.write(`  [wede] ${d}`));

  // Wait up to 8 s for it to become reachable
  const deadline = Date.now() + 8000;
  while (Date.now() < deadline) {
    await sleep(300);
    try {
      const res = await fetch(`${BASE_URL}/api/auth/check`);
      if (res.ok || res.status === 401) break;
    } catch (_) {}
  }

  return () => {
    proc.kill('SIGTERM');
  };
}

// ── main ──────────────────────────────────────────────────────────────────────

async function run() {
  console.log(`\nwede screenshotter`);
  console.log(`  BASE_URL  : ${BASE_URL}`);
  console.log(`  workspace : ${DEMO_WORKSPACE}`);
  console.log(`  output    : ${OUT_DIR}\n`);

  // Ensure the demo workspace has a real git history before starting wede
  setupDemoWorkspaceGit();

  const stopWede = await maybeStartWede();

  // Confirm wede is reachable after potential auto-start
  try {
    const res = await fetch(`${BASE_URL}/api/auth/check`);
    if (!res.ok && res.status !== 401) throw new Error(`HTTP ${res.status}`);
  } catch (err) {
    const note = [
      '# Screenshots — wede not reachable',
      '',
      `Could not connect to wede at ${BASE_URL}.`,
      '',
      'To capture screenshots:',
      '1. Start wede: `wede scripts/demo-workspace`',
      '2. Run: `npm run screenshots`',
      '',
      `Error: ${err.message}`,
    ].join('\n');
    writeFileSync(resolve(OUT_DIR, 'README.md'), note);
    console.error(`  ✗  wede not reachable at ${BASE_URL}`);
    console.error(`     Start wede first, then re-run npm run screenshots`);
    console.error(`     Wrote docs/screenshots/README.md with instructions.`);
    stopWede();
    process.exit(0); // exit 0 so CI is not broken
  }

  // Obtain a session token via the API before launching the browser
  let loginToken;
  try {
    const res = await fetch(`${BASE_URL}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password: PASSWORD }),
    });
    const data = await res.json();
    if (!data.token) throw new Error(data.error || data.message || 'login failed');
    loginToken = data.token;
    console.log(`  Authenticated (token: ${loginToken.slice(0, 12)}...)\n`);
  } catch (err) {
    console.error(`  ✗  Login failed: ${err.message}`);
    console.error(`     Check WEDE_PASSWORD matches the password in wede.config.json`);
    stopWede();
    process.exit(1);
  }

  const browser = await chromium.launch({ headless: true });

  // ── 1. Login screen — separate context with no token ─────────────────────
  console.log('Capturing: login screen...');
  {
    const ctx = await browser.newContext({ viewport: VIEWPORT });
    const page = await ctx.newPage();
    page.on('console', () => {});
    page.on('pageerror', () => {});
    // Seed theme only (so we skip ThemePicker but land on Login)
    await ctx.addInitScript((theme) => {
      localStorage.setItem('wede_theme', theme);
    }, THEME);
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[placeholder="Enter password"]', { timeout: 8000 });
    await sleep(400);
    await shot(page, 'login');
    await ctx.close();
  }

  // ── Create IDE context with pre-seeded auth token ─────────────────────────
  const ctx = await browser.newContext({ viewport: VIEWPORT });
  await ctx.addInitScript(({ tok, theme }) => {
    localStorage.setItem('wede_theme', theme);
    localStorage.setItem('wede_token', tok);
    localStorage.setItem('wede_username', 'Ava Chen');
    localStorage.setItem('wede_role', 'owner');
  }, { tok: loginToken, theme: THEME });

  const page = await ctx.newPage();
  page.on('console', () => {});
  page.on('pageerror', () => {});

  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await waitForIDE(page);

  // Collect sidebar button titles for use throughout
  const sidebarBtns = page.locator('button[title]');
  const btnCount = await sidebarBtns.count();

  // ── helper: click a sidebar button by title regex ─────────────────────────
  async function clickSidebar(regex) {
    for (let i = 0; i < btnCount; i++) {
      const title = await sidebarBtns.nth(i).getAttribute('title').catch(() => '');
      if (title && regex.test(title)) {
        await sidebarBtns.nth(i).click();
        await sleep(500);
        return true;
      }
    }
    return false;
  }

  // ── 2. IDE hero — editor open on api/handlers.go ─────────────────────────
  console.log('Capturing: IDE hero (editor + file tree)...');
  // The file explorer is open by default — do NOT click its activity button
  // (clicking the already-active tab collapses the sidebar). Just open a file.
  await sleep(400);

  // Tree rows are <div class="cursor-pointer"> with the name text; target by
  // exact visible text and let the click bubble to the row handler.
  const apiFolder = page.getByText('api', { exact: true }).first();
  if (await apiFolder.count() > 0) { await apiFolder.click(); await sleep(600); }

  async function openFileInTree(name) {
    const f = page.getByText(name, { exact: true }).first();
    if (await f.count() === 0) return false;
    await f.click();
    await page.waitForSelector('.cm-editor', { timeout: 6000 }).catch(() => {});
    await sleep(900);
    return true;
  }
  (await openFileInTree('handlers.go')) ||
    (await openFileInTree('main.go')) ||
    (await openFileInTree('middleware.go')) ||
    (await openFileInTree('README.md'));
  await sleep(300);
  await shot(page, 'hero');

  // ── 3. Git panel — Changes tab (unstaged diff for api/middleware.go) ──────
  console.log('Capturing: git panel (changes + diff)...');
  await clickSidebar(/source.?control/i); // activity button is titled "Source Control"
  await page.waitForFunction(() =>
    /Changes|Staged|Commit|modified|middleware/.test(document.body.innerText),
    { timeout: 5000 }
  ).catch(() => {});
  await sleep(700);
  // Reveal a file diff if a changed file is listed.
  const changedFile = page.locator('button, [class*="cursor-pointer"]').filter({ hasText: /\.(go|js|jsx|md)$/ }).first();
  if (await changedFile.count() > 0) { await changedFile.click().catch(() => {}); await sleep(700); }
  await shot(page, 'git');

  // ── 4. Git graph — History tab ────────────────────────────────────────────
  console.log('Capturing: git graph (commit history)...');
  const historyTab = page.locator('button').filter({ hasText: /^(History|Graph|Log|Commits)$/ }).first();
  if (await historyTab.count() > 0) { await historyTab.click(); await sleep(1400); }
  await shot(page, 'git_graph');

  // ── 5. Search panel — results for "handleCreate" ─────────────────────────
  console.log('Capturing: search panel...');
  await page.keyboard.press('Control+Shift+F');
  await sleep(700);
  const searchInput = page.locator('input[placeholder*="Search" i], input[placeholder*="Find" i]').first();
  if (await searchInput.count() > 0) {
    await searchInput.fill('handler');
    await searchInput.press('Enter');
    await sleep(1200); // wait for ripgrep results
  }
  await shot(page, 'search');
  await page.keyboard.press('Escape');
  await sleep(300);

  // ── 6. Terminal panel — show a real command ───────────────────────────────
  console.log('Capturing: terminal...');
  // Show the file tree in the sidebar (cleaner than a blank search panel),
  // then focus the terminal and run a command that produces visible output.
  // (Do NOT toggle the terminal button — that would hide the panel.)
  await clickSidebar(/explorer/i);
  await sleep(400);
  // Wait for the shell prompt to render (PTY connected) before typing.
  await page.waitForFunction(() => {
    const r = document.querySelector('.xterm-rows');
    return r && r.textContent && r.textContent.trim().length > 0;
  }, { timeout: 8000 }).catch(() => {});
  const termArea = page.locator('.xterm').first();
  if (await termArea.count() > 0) {
    await termArea.click({ force: true }).catch(() => {});
    // xterm routes keystrokes through a hidden textarea; focus it explicitly.
    await page.locator('.xterm-helper-textarea').focus().catch(() => {});
    await sleep(400);
    await page.keyboard.type('git log --oneline -5', { delay: 30 });
    await page.keyboard.press('Enter');
    await sleep(1800);
  }
  await shot(page, 'terminal');

  // ── 6b. Multiple terminals — per-project terminal manager (tabs) ───────────
  console.log('Capturing: multiple terminals...');
  const newTermBtn = page.locator('button[title="New Terminal"]').first();
  for (let i = 0; i < 2 && (await newTermBtn.count()) > 0; i++) {
    await newTermBtn.click();
    await sleep(900); // let the new PTY connect + render its prompt
  }
  const lastTerm = page.locator('.xterm-screen').last();
  if (await lastTerm.count() > 0) {
    await lastTerm.click({ force: true }).catch(() => {});
    await sleep(400);
    await page.keyboard.type('go build ./... && echo build ok', { delay: 22 });
    await page.keyboard.press('Enter');
    await sleep(1400);
  }
  await shot(page, 'terminals');

  // ── 7. Settings panel ─────────────────────────────────────────────────────
  console.log('Capturing: settings...');
  let settingsOpened = await clickSidebar(/settings/i);
  if (!settingsOpened) {
    await page.keyboard.press('Control+,');
    await sleep(600);
  }
  await shot(page, 'settings');
  await page.keyboard.press('Escape');
  await sleep(300);

  // ── 8. Command palette ────────────────────────────────────────────────────
  console.log('Capturing: command palette...');
  await page.keyboard.press('Control+Shift+P');
  await sleep(700);
  await page.waitForFunction(() =>
    document.body.innerText.includes('New File') ||
    document.body.innerText.includes('Toggle Terminal') ||
    document.body.innerText.includes('Save All'),
    { timeout: 4000 }
  ).catch(() => {});
  // Type a query so the palette shows filtered results
  await page.keyboard.type('git');
  await sleep(400);
  await shot(page, 'command_palette');
  await page.keyboard.press('Escape');
  await sleep(300);

  // ── 9. Browser preview — load wikipedia.org in the in-app preview ──────────
  console.log('Capturing: browser preview (wikipedia.org)...');
  await clickSidebar(/browser/i); // "Open Browser Preview" (Globe)
  await sleep(800);
  const urlInput = page.locator('input[placeholder*="URL" i]').first();
  if (await urlInput.count() > 0) {
    await urlInput.fill('https://www.wikipedia.org');
    await urlInput.press('Enter');
    await sleep(4000); // let the page load inside the iframe
  }
  await shot(page, 'browser');

  // ── 10. Chat — public / private channels + git activity ────────────────────
  console.log('Capturing: chat (public/private)...');
  await clickSidebar(/chat/i); // "Chat" activity button (MessageSquare)
  await sleep(1000);
  const chatInput = page.locator('textarea[placeholder*="Message" i]').first();
  if (await chatInput.count() > 0) {
    await chatInput.fill('Pushed the auth fix — terminal works now ✅');
    await chatInput.press('Enter');
    await sleep(500);
    await chatInput.fill('Nice. Reviewing the share links next.');
    await chatInput.press('Enter');
    await sleep(900);
  }
  await shot(page, 'chat');

  // ── 11. API client — collections in the sidebar + request/response editor ──
  console.log('Capturing: API client...');
  // Open the API collections in the sidebar (activity-bar button, exact title).
  await page.locator('button[title="API Client"]').first().click().catch(() => {});
  await sleep(900);
  // Open a saved request from the collections → request editor tab.
  await page.getByText('list-tasks', { exact: true }).first().click().catch(() => {});
  await sleep(1000);
  // Point it at a real API and send.
  const apiUrl = page.locator('input[placeholder*="api.example" i]').first();
  if (await apiUrl.count() > 0) {
    await apiUrl.fill('https://api.github.com/repos/vul-os/wede');
    await sleep(200);
    const sendBtn = page.locator('button:has-text("Send")').first();
    if (await sendBtn.count() > 0) {
      await sendBtn.click({ timeout: 5000 }).catch(() => {});
      await sleep(3500); // wait for the response
    }
  }
  await shot(page, 'apiclient');

  await browser.close();
  stopWede();
  console.log(`\nDone! Screenshots written to docs/screenshots/\n`);
}

run().catch(err => {
  console.error('\nScreenshotter error:', err.message);
  process.exit(1);
});
