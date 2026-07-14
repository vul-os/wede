#!/usr/bin/env node
// Format the JSON output of license-checker-rseidelsohn into an attribution
// section: name, version, licence id and the FULL licence text of every npm
// package bundled into the shipped web app.
//
// Usage:  npx license-checker-rseidelsohn --production --json --excludePrivatePackages --start . \
//           | node scripts/notices/npm-notices.mjs
//
// The package list comes from the real installed dependency tree — it is never
// hand-maintained. A package's licence text is taken, in order:
//   1. from an override file in scripts/notices/npm-license-overrides/ (used when
//      upstream ships NO licence file — the override holds the real upstream text
//      with a note recording where it was verified), matched by exact
//      "<name>@<version>", then "<name>", then a "<name-prefix>" family match;
//   2. from the LICENSE/COPYING file license-checker found in the package.
// If neither yields text, the script FAILS LOUDLY so a missing attribution can
// never be silently shipped.
import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = dirname(fileURLToPath(import.meta.url));
const OVERRIDE_DIR = join(HERE, 'npm-license-overrides');

// Load overrides. Filenames encode the package name with "/" -> "__" and drop
// any trailing "@version"; the file's first "Licence:" line names the id.
const overrides = [];
if (existsSync(OVERRIDE_DIR)) {
  for (const f of readdirSync(OVERRIDE_DIR)) {
    if (!f.endsWith('.txt')) continue;
    const key = f.slice(0, -4).replace(/__/g, '/'); // e.g. "@rolldown/binding"
    overrides.push({ key, text: readFileSync(join(OVERRIDE_DIR, f), 'utf8').trimEnd() });
  }
}

function findOverride(name, version) {
  // exact name@version, then name, then longest prefix (for platform families
  // like @rolldown/binding-darwin-arm64 matching an "@rolldown/binding" file).
  return (
    overrides.find((o) => o.key === `${name}@${version}`) ||
    overrides.find((o) => o.key === name) ||
    overrides
      .filter((o) => name.startsWith(o.key))
      .sort((a, b) => b.key.length - a.key.length)[0] ||
    null
  );
}

const raw = readFileSync(0, 'utf8');
const pkgs = JSON.parse(raw);

const LICENCE_FILE = /^(licen[cs]e|copying|notice)/i;
const out = [];
const problems = [];

for (const pkgKey of Object.keys(pkgs).sort()) {
  const info = pkgs[pkgKey];
  const at = pkgKey.lastIndexOf('@');
  const name = pkgKey.slice(0, at);
  const version = pkgKey.slice(at + 1);
  const licence = Array.isArray(info.licenses) ? info.licenses.join(' OR ') : info.licenses;

  let text = null;
  let source = '';

  const ov = findOverride(name, version);
  if (ov) {
    text = ov.text;
    source = ` (licence text: override for ${ov.key})`;
  } else {
    const file = info.licenseFile;
    if (file && LICENCE_FILE.test(basename(file))) {
      try {
        text = readFileSync(file, 'utf8').trimEnd();
      } catch (err) {
        problems.push(`${pkgKey}: cannot read ${file}: ${err.message}`);
        continue;
      }
    } else {
      problems.push(
        `${pkgKey}: no licence file found (licenseFile=${file || 'none'}) and no override in scripts/notices/npm-license-overrides/`,
      );
      continue;
    }
  }

  out.push(
    '-'.repeat(80),
    `Package : ${name}`,
    `Version : ${version}`,
    `Licence : ${licence}${source}`,
    '-'.repeat(80),
    '',
    text,
    '',
  );
}

if (problems.length) {
  console.error('npm-notices: cannot attribute the following packages:');
  for (const p of problems) console.error('  - ' + p);
  console.error('Add the real upstream licence text as an override in');
  console.error('scripts/notices/npm-license-overrides/<name-with-__-for-slash>.txt');
  process.exit(1);
}

process.stdout.write(out.join('\n') + '\n');
