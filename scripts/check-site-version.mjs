#!/usr/bin/env node
// Every version string a reader can see must equal web/package.json's version.
//
// release.yml already refuses to build a tag whose name disagrees with
// web/package.json, so the tag and the binary can never diverge. Nothing
// checked the *pages*, though, and they did diverge: the landing, the docs
// header and the README's verify.sh example all advertised v0.6.0 — a version
// tagged off an unpublished milestone and since deleted — while the newest
// real download was v0.1.2. A visitor following the README would have fetched
// a tag that did not exist.
//
// Two design notes, both learned by getting this wrong first:
//
// 1. Do NOT hunt for version-shaped strings. A `\bv?\d+\.\d+\.\d+\b` sweep of
//    these files matches 34 things in site/index.html alone — SVG path data
//    ("v9.73.5"), listen addresses (127.0.0.1, 169.254.169.254) and 0.0.0.0.
//    The published versions are therefore *marked* at their source, with a
//    `data-wede-version` attribute in HTML, and matched by their surrounding
//    syntax in README.md where an HTML comment would render literally inside
//    the bash fence.
//
// 2. Do NOT just check that the versions found are correct. That form cannot
//    fail usefully: delete the version from the page and the check still
//    passes, because "no wrong versions" and "no versions at all" are the same
//    observation. Each file's expected *count* is asserted too, so dropping a
//    marker is a failure, and adding one deliberately means editing EXPECTED.

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const expected = JSON.parse(
  readFileSync(join(root, 'web/package.json'), 'utf8'),
).version;

// Any element carrying `data-wede-version`; the version is the first
// version-shaped token in its text.
const MARKED_HTML = /<[^>]*\bdata-wede-version\b[^>]*>([^<]*)</g;
// README quotes the tag to download in a bash example.
const README_TAG = /--tag\s+v(\d+\.\d+\.\d+)/g;

const FILES = [
  { rel: 'site/index.html', re: MARKED_HTML, count: 3 },
  { rel: 'site/docs.html', re: MARKED_HTML, count: 1 },
  { rel: 'README.md', re: README_TAG, count: 1 },
];

let failed = false;
const fail = (msg) => {
  console.error(`  ✗ ${msg}`);
  failed = true;
};

for (const { rel, re, count } of FILES) {
  const text = readFileSync(join(root, rel), 'utf8');
  const found = [];

  for (const m of text.matchAll(re)) {
    const version = (m[1].match(/\d+\.\d+\.\d+/) || [])[0];
    const line = text.slice(0, m.index).split('\n').length;
    found.push({ line, version, raw: m[1].trim() });
  }

  if (found.length !== count) {
    fail(
      `${rel}: found ${found.length} published version(s), expected ${count}` +
        ` — [${found.map((f) => `L${f.line} "${f.raw}"`).join(', ')}].` +
        ` If intentional, update FILES in scripts/check-site-version.mjs.`,
    );
    continue;
  }

  for (const f of found) {
    if (f.version !== expected) {
      fail(
        `${rel}:${f.line}: shows "${f.raw}" (${f.version ?? 'no version'}),` +
          ` but web/package.json says ${expected}`,
      );
    }
  }

  console.log(`  ✓ ${rel}: ${found.length} published version(s)`);
}

if (failed) {
  console.error(`\ncheck-site-version: FAIL — expected every published version to be ${expected}`);
  process.exit(1);
}
console.log(`\ncheck-site-version: every published version reads ${expected}`);
