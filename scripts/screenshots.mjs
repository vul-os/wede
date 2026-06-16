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
 */

import { chromium } from 'playwright';
import { mkdirSync, writeFileSync } from 'fs';
import { resolve, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = resolve(__dirname, '..');
const OUT_DIR = resolve(ROOT, 'docs', 'screenshots');

const BASE_URL = process.env.BASE_URL || 'http://localhost:9090';
const PASSWORD = process.env.WEDE_PASSWORD || 'admin';
const VIEWPORT = { width: 1440, height: 900 };

mkdirSync(OUT_DIR, { recursive: true });

// ── helpers ───────────────────────────────────────────────────────────────────

async function shot(page, name) {
  const file = resolve(OUT_DIR, `${name}.png`);
  await page.screenshot({ path: file, fullPage: false });
  console.log(`  ✓  ${name}.png`);
}

async function sleep(ms) {
  return new Promise(r => setTimeout(r, ms));
}

async function waitForIDE(page) {
  // Wait for file explorer content or editor to appear
  await page.waitForFunction(() =>
    // File tree visible
    document.body.innerText.includes('WEDE') ||
    document.querySelector('.cm-editor') ||
    document.body.innerText.length > 200,
    { timeout: 12000 }
  ).catch(() => {});
  await sleep(800);
}

// ── main ──────────────────────────────────────────────────────────────────────

async function run() {
  console.log(`\nwede screenshotter`);
  console.log(`  BASE_URL : ${BASE_URL}`);
  console.log(`  output   : ${OUT_DIR}\n`);

  // Check wede is reachable
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
      '1. Start wede: `wede /path/to/project`',
      '2. Run: `npm run screenshots`',
      '',
      `Error: ${err.message}`,
    ].join('\n');
    writeFileSync(resolve(OUT_DIR, 'README.md'), note);
    console.error(`  ✗  wede not reachable at ${BASE_URL}`);
    console.error(`     Start wede first, then re-run npm run screenshots`);
    console.error(`     Wrote docs/screenshots/README.md with instructions.`);
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
    await ctx.addInitScript(() => {
      localStorage.setItem('wede_theme', 'dark');
    });
    // SSE will block networkidle — use domcontentloaded + wait
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('input[placeholder="Enter password"]', { timeout: 8000 });
    await sleep(400);
    await shot(page, 'login');
    await ctx.close();
  }

  // ── Create IDE context with pre-seeded auth token ─────────────────────────
  const ctx = await browser.newContext({ viewport: VIEWPORT });
  // Use addInitScript so localStorage is set before any JS runs
  // Note: wede_theme must be set first to skip ThemePicker; then wede_token
  // to skip Login. The SSE /api/watch connection keeps networkidle open, so
  // always use waitUntil:'domcontentloaded' and then wait for the app.
  await ctx.addInitScript(({ tok }) => {
    localStorage.setItem('wede_theme', 'dark');
    localStorage.setItem('wede_token', tok);
  }, { tok: loginToken });

  const page = await ctx.newPage();
  page.on('console', () => {});
  page.on('pageerror', () => {});

  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await waitForIDE(page);

  // ── 2. IDE hero — editor + file tree ─────────────────────────────────────
  console.log('Capturing: IDE hero...');
  // Click the first non-directory item in the file tree to open an editor
  const fileTree = page.locator('li, [role="treeitem"]').filter({ hasText: /\.(js|jsx|go|ts|json|md)$/i }).first();
  if (await fileTree.count() > 0) {
    await fileTree.click();
    await page.waitForSelector('.cm-editor', { timeout: 5000 }).catch(() => {});
    await sleep(600);
  }
  await shot(page, 'hero');

  // ── 3. Git panel — Changes tab ────────────────────────────────────────────
  console.log('Capturing: git panel...');
  // Find sidebar buttons with titles and click the Git one
  const sidebarBtns = page.locator('button[title]');
  const btnCount = await sidebarBtns.count();
  let gitOpened = false;
  for (let i = 0; i < btnCount; i++) {
    const title = await sidebarBtns.nth(i).getAttribute('title').catch(() => '');
    if (title && /git/i.test(title)) {
      await sidebarBtns.nth(i).click();
      await sleep(700);
      gitOpened = true;
      break;
    }
  }
  if (!gitOpened) {
    // Fallback: second sidebar button is usually git
    if (btnCount > 1) { await sidebarBtns.nth(1).click(); await sleep(700); }
  }
  await shot(page, 'git');

  // ── 4. Git graph — History tab ────────────────────────────────────────────
  console.log('Capturing: git graph...');
  const historyTab = page.locator('button:has-text("History")');
  if (await historyTab.count() > 0) {
    await historyTab.first().click();
    await sleep(1000); // wait for SVG graph to render
  }
  await shot(page, 'git_graph');

  // ── 5. Search panel ───────────────────────────────────────────────────────
  console.log('Capturing: search panel...');
  // Open files sidebar first (to return to normal state)
  for (let i = 0; i < btnCount; i++) {
    const title = await sidebarBtns.nth(i).getAttribute('title').catch(() => '');
    if (title && /file|explorer/i.test(title)) {
      await sidebarBtns.nth(i).click();
      await sleep(300);
      break;
    }
  }
  await page.keyboard.press('Control+Shift+F');
  await sleep(500);
  const searchInput = page.locator('input[placeholder*="Search" i], input[placeholder*="Find" i]').first();
  if (await searchInput.count() > 0) {
    await searchInput.fill('function');
    await sleep(900);
  }
  await shot(page, 'search');
  await page.keyboard.press('Escape');
  await sleep(300);

  // ── 6. Terminal panel ─────────────────────────────────────────────────────
  console.log('Capturing: terminal...');
  // Find terminal button in sidebar/toolbar
  let termOpened = false;
  for (let i = 0; i < btnCount; i++) {
    const title = await sidebarBtns.nth(i).getAttribute('title').catch(() => '');
    if (title && /terminal/i.test(title)) {
      await sidebarBtns.nth(i).click();
      await sleep(800);
      termOpened = true;
      break;
    }
  }
  if (!termOpened) {
    // Ctrl+` shortcut
    await page.keyboard.press('Control+`');
    await sleep(800);
  }
  await page.waitForFunction(() =>
    document.querySelector('.xterm-screen, .xterm, [class*="terminal"]'),
    { timeout: 5000 }
  ).catch(() => {});
  await sleep(400);
  await shot(page, 'terminal');

  // ── 7. Settings panel ─────────────────────────────────────────────────────
  console.log('Capturing: settings...');
  let settingsOpened = false;
  for (let i = 0; i < btnCount; i++) {
    const title = await sidebarBtns.nth(i).getAttribute('title').catch(() => '');
    if (title && /settings/i.test(title)) {
      await sidebarBtns.nth(i).click();
      await sleep(600);
      settingsOpened = true;
      break;
    }
  }
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
  await sleep(600);
  await page.waitForFunction(() =>
    document.body.innerText.includes('New File') ||
    document.body.innerText.includes('Toggle Terminal') ||
    document.body.innerText.includes('Save All'),
    { timeout: 4000 }
  ).catch(() => {});
  await sleep(300);
  await shot(page, 'command_palette');
  await page.keyboard.press('Escape');

  await browser.close();
  console.log(`\nDone! Screenshots written to docs/screenshots/\n`);
}

run().catch(err => {
  console.error('\nScreenshotter error:', err.message);
  process.exit(1);
});
