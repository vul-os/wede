#!/usr/bin/env node
/**
 * sync-docs.mjs — generate site/docs/*.md from the repo's canonical docs.
 *
 * The website's docs viewer (site/docs.html) fetches ./docs/<slug>.md. Those
 * files used to be hand-maintained copies of docs/*.md, README.md, ROADMAP.md
 * and CHANGELOG.md — which drifted badly (a stale roadmap, a changelog two
 * releases behind, and a copy of ARCHITECTURE.md that had silently dropped the
 * no-sandbox security warning). There is now exactly one source of truth per
 * page, and this script derives the site copies from it.
 *
 * Run it via `npm run docs:sync`; `npm run build:all` runs it first.
 *
 * Transforms applied to each file:
 *   - repo-relative image paths (docs/screenshots/…, docs/assets/…) are
 *     rewritten to the paths the site actually serves
 *   - cross-document .md links are rewritten to the viewer's hash routes
 *     (#slug, or #slug:anchor when the link carried a heading anchor)
 *   - links to files that are not published on the site point at GitHub
 *   - a generated-file banner is prepended as an HTML comment
 */

import { readFileSync, writeFileSync, mkdirSync, readdirSync, unlinkSync } from 'node:fs';
import { dirname, join, resolve, basename } from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const OUT = join(ROOT, 'site', 'docs');
const REPO = 'https://github.com/vul-os/wede/blob/main/';

/** Source of truth → published slug. Order matches the docs.html nav. */
const PAGES = [
  { slug: 'overview',           src: 'docs/OVERVIEW.md' },
  { slug: 'getting-started',    src: 'docs/GETTING-STARTED.md' },
  { slug: 'configuration',      src: 'docs/CONFIGURATION.md' },
  { slug: 'deployment',         src: 'docs/DEPLOYMENT.md' },
  { slug: 'public-access',      src: 'docs/PUBLIC-ACCESS.md' },
  { slug: 'security-hardening', src: 'docs/SECURITY-HARDENING.md' },
  { slug: 'troubleshooting',    src: 'docs/TROUBLESHOOTING.md' },
  { slug: 'architecture',       src: 'docs/ARCHITECTURE.md' },
  { slug: 'api',                src: 'docs/API.md' },
  { slug: 'screenshots',        src: 'docs/SCREENSHOTS.md' },
  { slug: 'roadmap',            src: 'ROADMAP.md' },
  { slug: 'changelog',          src: 'CHANGELOG.md' },
];

/** Basename (lowercased) → slug, for rewriting cross-document links. */
const BY_FILE = new Map(PAGES.map((p) => [basename(p.src).toLowerCase(), p.slug]));
BY_FILE.set('readme.md', 'overview');
BY_FILE.set('security.md', 'security-hardening');

const BANNER = (src) =>
  `<!-- Generated from ${src} by scripts/sync-docs.mjs — edit the source, not this file. -->\n\n`;

/** Rewrite one link/image target. Returns the replacement href. */
function rewriteTarget(href, { isImage }) {
  if (/^(https?:)?\/\//i.test(href) || /^mailto:/i.test(href) || href.startsWith('#')) return href;

  const bare = href.replace(/^(?:\.\.\/)+/, '');

  if (isImage) {
    const shot = bare.match(/^(?:docs\/)?screenshots\/(.+)$/i);
    if (shot) return `screenshots/${shot[1]}`;
    const asset = bare.match(/^(?:docs\/)?assets\/(.+)$/i);
    if (asset) return `assets/${asset[1]}`;
    return REPO.replace('/blob/', '/raw/') + bare;
  }

  const md = bare.match(/^([^/#?]+\.md)(?:#(.+))?$/i) || bare.match(/^docs\/([^/#?]+\.md)(?:#(.+))?$/i);
  if (md) {
    const file = md[1].toLowerCase();
    const slug = BY_FILE.get(file);
    // README/SECURITY are folded into other pages, so their own section anchors
    // do not exist on the site — send anchored links to the file on GitHub.
    if (slug && !(md[2] && (file === 'readme.md' || file === 'security.md'))) {
      return `#${slug}${md[2] ? `:${md[2]}` : ''}`;
    }
    return REPO + bare;
  }

  return REPO + bare;
}

function transform(md) {
  let out = md;

  // ![alt](path) and [text](path)
  out = out.replace(/(!?)\[([^\]]*)\]\(([^)\s]+)(\s+"[^"]*")?\)/g, (m, bang, text, href, title) => {
    const next = rewriteTarget(href, { isImage: bang === '!' });
    return `${bang}[${text}](${next}${title || ''})`;
  });

  // <img src="…"> written as raw HTML
  out = out.replace(/(<img\b[^>]*\bsrc=")([^"]+)(")/gi, (m, a, href, b) =>
    a + rewriteTarget(href, { isImage: true }) + b);

  // <a href="…"> written as raw HTML
  out = out.replace(/(<a\b[^>]*\bhref=")([^"]+)(")/gi, (m, a, href, b) =>
    a + rewriteTarget(href, { isImage: false }) + b);

  return out;
}

mkdirSync(OUT, { recursive: true });

// Drop previously generated pages that are no longer in PAGES, so a renamed or
// retired doc cannot linger on the site.
const keep = new Set(PAGES.map((p) => `${p.slug}.md`));
for (const f of readdirSync(OUT)) {
  if (f.endsWith('.md') && !keep.has(f)) {
    unlinkSync(join(OUT, f));
    console.log(`  removed  site/docs/${f} (no longer published)`);
  }
}

let missing = 0;
for (const page of PAGES) {
  const from = join(ROOT, page.src);
  let raw;
  try {
    raw = readFileSync(from, 'utf8');
  } catch {
    console.error(`  MISSING  ${page.src} → site/docs/${page.slug}.md`);
    missing++;
    continue;
  }
  writeFileSync(join(OUT, `${page.slug}.md`), BANNER(page.src) + transform(raw));
  console.log(`  synced   ${page.src} → site/docs/${page.slug}.md`);
}

if (missing) {
  console.error(`\nsync-docs: ${missing} source file(s) missing — the site will 404 on those pages.`);
  process.exit(1);
}
console.log(`\nsync-docs: ${PAGES.length} pages written to site/docs/`);
