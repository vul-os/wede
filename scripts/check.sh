#!/usr/bin/env bash
# Single verification gate for the collaborative-IDE rebuild.
# Every wave cycle must end with this passing.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

fail=0
step() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }

step "backend: go build"
( cd backend && go build ./... ) || fail=1

step "backend: go test"
( cd backend && go test ./... ) || fail=1

step "frontend: lint"
( cd web && npm run lint ) || fail=1

step "frontend: test"
( cd web && npm test ) || fail=1

step "frontend: build"
( cd web && npm run build ) || fail=1

# The release guards are part of the gate, not a release-day afterthought. Both
# assert that the refusals still fire — a checksum gate that has quietly
# stopped failing looks exactly like one that works until someone is shipped a
# substituted binary.
# The landing page and docs are rendered in a real browser at eight viewports
# in both colour schemes. Nothing here is visible in a diff: a stretched
# screenshot, a light-mode capture shown in dark mode, copy under 12px, an
# element pushing the page sideways. --selftest breaks each invariant on
# purpose first, so a gate that has stopped discriminating fails loudly rather
# than reporting clean.
step "site: render checks"
if [ -d scripts/node_modules/playwright ] || npm --prefix scripts ls playwright >/dev/null 2>&1; then
  node scripts/check-render.mjs --selftest || fail=1
  node scripts/check-render.mjs || fail=1
else
  printf 'playwright is not installed under scripts/ — run: npm --prefix scripts i playwright && npx playwright install chromium\n'
  fail=1
fi

step "release: verify.sh failure matrix"
bash scripts/verify.sh --selftest || fail=1

step "release: install.sh failure matrix"
bash scripts/install-failure-matrix.sh || fail=1

if [ "$fail" -ne 0 ]; then
  printf '\n\033[31mCHECK FAILED\033[0m\n'
  exit 1
fi
printf '\n\033[32mCHECK PASSED\033[0m\n'
