#!/usr/bin/env bash
# =============================================================================
# scripts/install-failure-matrix.sh — prove install.sh refuses, every way.
#
# install.sh is the script users pipe into a shell. Its checksum gate is only
# worth having if its refusals actually fire, so this stands up a synthetic
# release origin on loopback whose routes are each broken in exactly one way
# and asserts, for every case:
#
#   1. the exit status                (0 for the happy path, non-zero otherwise)
#   2. that a diagnostic was PRINTED  — "aborted with no message" was the real
#                                       bug class in the sibling installers
#   3. THAT NOTHING WAS INSTALLED     — the assertion that actually matters.
#                                       An installer can print a refusal and
#                                       still have left a binary on disk.
#
# Nothing here touches the network, the real ~/.local/bin, or the real
# ~/.config: HOME, WEDE_INSTALL_DIR and the origin URL are all redirected into
# a temporary directory that is deleted on exit. The "binary" served is a short
# text file and is never executed.
#
# Run:  bash scripts/install-failure-matrix.sh
# =============================================================================
set -euo pipefail

SELF_NAME="${BASH_SOURCE[0]##*/}"
SELF_DIR="${BASH_SOURCE[0]%/*}"
[ "$SELF_DIR" = "${BASH_SOURCE[0]}" ] && SELF_DIR="."
REPO_ROOT="$(cd "$SELF_DIR/.." && pwd)"
INSTALL_SH="${REPO_ROOT}/install.sh"

[ -f "$INSTALL_SH" ] || { echo "$SELF_NAME: no install.sh at $INSTALL_SH" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || {
  echo "$SELF_NAME: python3 is required (test-only dependency)" >&2; exit 1; }

if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
  GRN="$(tput setaf 2)"; BLD="$(tput bold)"; RST="$(tput sgr0)"
else
  GRN=''; BLD=''; RST=''
fi

TMP="$(mktemp -d "${TMPDIR:-/tmp}/wede-install-matrix.XXXXXX")"
SRV_PID=""
cleanup() {
  [ -n "$SRV_PID" ] && kill "$SRV_PID" 2>/dev/null
  rm -rf -- "$TMP"
  return 0
}
trap cleanup EXIT

# The asset name install.sh will ask for on THIS machine — derived the same way
# install.sh derives it, so the synthetic origin and the installer agree
# without either being told.
case "$(uname -s)" in
  Linux*)  T_OS="linux";  T_EXT="" ;;
  Darwin*) T_OS="darwin"; T_EXT="" ;;
  MINGW*|MSYS*|CYGWIN*) T_OS="windows"; T_EXT=".exe" ;;
  *) echo "$SELF_NAME: unsupported test host $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  T_ARCH="amd64" ;;
  aarch64|arm64) T_ARCH="arm64" ;;
  *) echo "$SELF_NAME: unsupported test host arch $(uname -m)" >&2; exit 1 ;;
esac
ASSET="wede-${T_OS}-${T_ARCH}${T_EXT}"
TAG="v9.9.9"

# ── The synthetic origin ─────────────────────────────────────────────────────
cat > "${TMP}/origin.py" <<'PYEOF'
import hashlib, sys, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ASSET = sys.argv[1]
TAG   = sys.argv[2]
MANIFEST = "checksums.txt"

GOOD  = b"#!/bin/false\n# synthetic wede binary, never executed\n" * 32
OTHER = b"#!/bin/false\n# SUBSTITUTED bytes, not the published ones\n" * 32
HTML  = (b"<!DOCTYPE html><html><head><title>404 Not Found</title></head>"
         b"<body><h1>Not Found</h1></body></html>")

def line(name, data):
    return "%s  %s\n" % (hashlib.sha256(data).hexdigest(), name)

good_sums = line(ASSET, GOOD).encode()

ROUTES = {}
def add(case, path, status, ctype, body, declared=None):
    ROUTES["/%s/download/%s/%s" % (case, TAG, path)] = (status, ctype, body, declared)

add("good",      ASSET,    200, "application/octet-stream", GOOD)
add("good",      MANIFEST, 200, "text/plain", good_sums)

# manifest 404 — the case that used to be a silent skip elsewhere in the suite
add("nosums",    ASSET,    200, "application/octet-stream", GOOD)

add("emptysums", ASSET,    200, "application/octet-stream", GOOD)
add("emptysums", MANIFEST, 200, "text/plain", b"")

add("htmlsums",  ASSET,    200, "application/octet-stream", GOOD)
add("htmlsums",  MANIFEST, 200, "text/plain", HTML)          # lying content-type

add("noentry",   ASSET,    200, "application/octet-stream", GOOD)
add("noentry",   MANIFEST, 200, "text/plain", line("wede-some-other-target", GOOD).encode())

# The .sig trap arranged so a SUBSTRING match falsely passes: the manifest
# vouches only for "<asset>.sig", and the origin serves those exact bytes under
# the asset's own name. Exact field-2 matching must refuse.
add("sigswap",   ASSET,    200, "application/octet-stream", OTHER)
add("sigswap",   MANIFEST, 200, "text/plain", line(ASSET + ".sig", OTHER).encode())

add("malformed", ASSET,    200, "application/octet-stream", GOOD)
add("malformed", MANIFEST, 200, "text/plain", ("deadbeef  %s\n" % ASSET).encode())

add("mismatch",  ASSET,    200, "application/octet-stream", OTHER)
add("mismatch",  MANIFEST, 200, "text/plain", good_sums)

# declares more bytes than it sends, then hangs up: curl exit 18
add("truncart",  ASSET,    200, "application/octet-stream", GOOD[:64], 50000)
add("truncart",  MANIFEST, 200, "text/plain", good_sums)

add("noart",     MANIFEST, 200, "text/plain", good_sums)

class H(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    def log_message(self, *a):
        pass
    def _send(self, status, ctype, body, declared=None):
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body) if declared is None else declared))
        self.end_headers()
        try:
            self.wfile.write(body)
            self.wfile.flush()
        except Exception:
            pass
        if declared is not None:
            self.close_connection = True
    def _redirect(self, to):
        self.send_response(302)
        self.send_header("Location", to)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def do_HEAD(self):
        # The "no releases yet" case MUST be matched before the generic
        # /latest rule below, or it never fires and its test silently becomes
        # a duplicate of another case.
        if self.path.startswith("/notag/"):
            if self.path.endswith("/releases"):
                self.send_response(200)
                self.send_header("Content-Type", "text/html")
                self.send_header("Content-Length", "0")
                self.end_headers()
                return
            # GitHub redirects /releases/latest to the releases index when a
            # repo has no releases: the effective URL carries no tag.
            self._redirect("/notag/releases")
            return
        # install.sh resolves "latest" by following the redirect and reading the
        # tag off the effective URL. /<case>/latest -> /<case>/tag/<TAG>.
        if self.path.endswith("/latest"):
            self._redirect(self.path[:-len("latest")] + "tag/" + TAG)
            return
        if self.path.endswith("/tag/" + TAG):
            self.send_response(200)
            self.send_header("Content-Type", "text/html")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        self.send_response(404)
        self.send_header("Content-Length", "0")
        self.end_headers()
    def do_GET(self):
        r = ROUTES.get(self.path)
        if r is None:
            self._send(404, "text/html", HTML)
            return
        self._send(*r)

srv = ThreadingHTTPServer(("127.0.0.1", 0), H)
print("PORT %d" % srv.server_address[1], flush=True)
threading.Thread(target=srv.serve_forever, daemon=True).start()
try:
    threading.Event().wait()
except KeyboardInterrupt:
    pass
PYEOF

python3 "${TMP}/origin.py" "$ASSET" "$TAG" > "${TMP}/origin.log" 2>&1 &
SRV_PID=$!

PORT=""
for _ in $(seq 1 100); do
  PORT="$(awk '/^PORT /{print $2; exit}' "${TMP}/origin.log" 2>/dev/null || true)"
  [ -n "$PORT" ] && break
  sleep 0.1
done
[ -n "$PORT" ] || { echo "$SELF_NAME: synthetic origin failed to start:" >&2
                    cat "${TMP}/origin.log" >&2; exit 1; }
BASE="http://127.0.0.1:${PORT}"

# A PATH holding everything install.sh needs EXCEPT `gh`. Cases below run
# against this so the digest gate is measured on its own; the attestation gate
# gets its own case at the end, where `gh` being present is the point.
NOGH="${TMP}/nogh"
mkdir -p "$NOGH"
for _t in bash sh curl awk grep sed tr head tail cat wc mktemp mkdir rm mv ln \
          env uname chmod printf tput seq paste sha256sum shasum openssl basename; do
  _p="$(command -v "$_t" 2>/dev/null || true)"
  [ -n "$_p" ] && ln -sf "$_p" "${NOGH}/${_t}"
done
[ -e "${NOGH}/gh" ] && { echo "$SELF_NAME: gh leaked into the no-gh PATH" >&2; exit 1; }

FAILURES=0
N=0

# run_case <label> <expect-exit:0|nonzero> <installed:yes|no> <expect-substring> -- <env...> -- <base-path>
run_case() {
  local label="$1" want="$2" want_installed="$3" want_msg="$4"; shift 4
  [ "${1:-}" = "--" ] && shift
  local envs=()
  while [ "${1:-}" != "--" ] && [ $# -gt 0 ]; do envs+=("$1"); shift; done
  shift || true
  local url="$1"

  N=$((N + 1))
  local sandbox="${TMP}/case${N}"
  local bindir="${sandbox}/bin" fakehome="${sandbox}/home"
  mkdir -p "$bindir" "$fakehome"

  local outf="${sandbox}/out" rc=0
  # PATH is $NOGH: everything install.sh needs except `gh`, so provenance is
  # uniformly "not checked" here and this matrix measures the digest gate on
  # its own. The attestation gate has its own case below.
  env -i PATH="$NOGH" \
      HOME="$fakehome" \
      WEDE_INSTALL_DIR="$bindir" \
      WEDE_INSTALL_BASE_URL="$url" \
      WEDE_VERSION="${WEDE_VERSION_OVERRIDE:-}" \
      WEDE_REPO="vul-os/wede" \
      "${envs[@]}" \
      "${BASH:-/bin/bash}" "$INSTALL_SH" >"$outf" 2>&1 || rc=$?

  local installed="no"
  [ -e "${bindir}/wede${T_EXT}" ] && installed="yes"

  local diag
  diag="$(grep -m1 -E 'FATAL:' "$outf" | sed 's/.*FATAL: *//' || true)"
  [ -n "$diag" ] || diag="$(tail -1 "$outf" || true)"
  [ -n "$diag" ] || diag="(NO DIAGNOSTIC PRINTED)"

  local verdict="ok"
  if [ "$want" = "0" ] && [ "$rc" -ne 0 ]; then
    verdict="FAIL(exit ${rc}, want 0)"
  elif [ "$want" != "0" ] && [ "$rc" -eq 0 ]; then
    verdict="FAIL(exit 0, want non-zero)"
  elif [ "$installed" != "$want_installed" ]; then
    # The assertion that matters: a refusal that still left a binary behind is
    # not a refusal.
    verdict="FAIL(installed=${installed}, want ${want_installed})"
  elif [ "$want" != "0" ] && [ "$diag" = "(NO DIAGNOSTIC PRINTED)" ]; then
    verdict="FAIL(silent)"
  elif [ -n "$want_msg" ] && ! grep -qF -- "$want_msg" "$outf"; then
    verdict="FAIL(no '${want_msg}')"
  fi
  [ "$verdict" = "ok" ] || FAILURES=$((FAILURES + 1))

  printf '  %-32s exit %-4s installed=%-4s %-30s %s\n' \
    "$label" "$rc" "$installed" "$verdict" "$diag"
}

printf '\n%s%s — synthetic release origin on %s%s\n' "$BLD" "$SELF_NAME" "$BASE" "$RST"
printf '  asset under test: %s   tag: %s\n\n' "$ASSET" "$TAG"
printf '  %-32s %-9s %-14s %-30s %s\n' "CASE" "EXIT" "INSTALLED?" "VERDICT" "DIAGNOSTIC"
printf '  %s\n' "--------------------------------------------------------------------------------------------------------"

run_case "happy path"                0 yes "verified sha256"                    -- -- "${BASE}/good"
run_case "checksums.txt 404"         1 no  "refusing to install unverified"     -- -- "${BASE}/nosums"
run_case "checksums.txt empty"       1 no  "is empty (0 bytes)"                 -- -- "${BASE}/emptysums"
run_case "checksums.txt is HTML"     1 no  "HTML page where checksums.txt"      -- -- "${BASE}/htmlsums"
run_case "no entry for this asset"   1 no  "has no entry for"                   -- -- "${BASE}/noentry"
run_case "the .sig false-pass trap"  1 no  "has no entry for"                   -- -- "${BASE}/sigswap"
run_case "manifest digest malformed" 1 no  "not a 64-hex SHA-256 digest"        -- -- "${BASE}/malformed"
run_case "digest mismatch"           1 no  "CHECKSUM MISMATCH"                  -- -- "${BASE}/mismatch"
run_case "truncated download"        1 no  "TRUNCATED"                          -- -- "${BASE}/truncart"
run_case "binary 404"                1 no  "download failed"                    -- -- "${BASE}/noart"

# Transport / environment refusals.
run_case "plaintext non-loopback"    1 no  "refusing a plaintext"               -- -- "http://example.com/releases"
WEDE_VERSION_OVERRIDE="" \
run_case "no resolvable release tag" 1 no  "no release tag could be read"       -- -- "${BASE}/notag"

# The attestation gate. On a machine WITH the gh CLI, install.sh additionally
# demands sigstore build provenance — and a synthetic origin has none, so even
# a digest that matches perfectly must not install. This is the case that
# proves --attest-equivalent behaviour is fail-closed rather than best-effort.
# It is reported as SKIP, never as a pass, when gh is absent: a case that did
# not run must never be counted as a case that succeeded.
if command -v gh >/dev/null 2>&1; then
  ATT_HOME="${TMP}/att-home"; ATT_BIN="${TMP}/att-bin"
  mkdir -p "$ATT_HOME" "$ATT_BIN"
  rc=0
  env HOME="$ATT_HOME" \
      WEDE_INSTALL_DIR="$ATT_BIN" \
      WEDE_INSTALL_BASE_URL="${BASE}/good" \
      WEDE_VERSION="$TAG" \
      "${BASH:-/bin/bash}" "$INSTALL_SH" > "${TMP}/att.out" 2>&1 || rc=$?
  installed="no"; [ -e "${ATT_BIN}/wede${T_EXT}" ] && installed="yes"
  diag="$(grep -m1 -E 'FATAL:' "${TMP}/att.out" | sed 's/.*FATAL: *//' || true)"
  [ -n "$diag" ] || diag="(NO DIAGNOSTIC PRINTED)"
  verdict="ok"
  if [ "$rc" -eq 0 ] || [ "$installed" != "no" ] ||
     ! grep -qF "build provenance attestation FAILED" "${TMP}/att.out"; then
    verdict="FAIL"
    FAILURES=$((FAILURES + 1))
  fi
  N=$((N + 1))
  printf '  %-32s exit %-4s installed=%-4s %-30s %s\n' \
    "gh present, no attestation" "$rc" "$installed" "$verdict" "$diag"
else
  printf '  %-32s %s\n' "gh present, no attestation" \
    "SKIP (gh not installed — case did not run, and is not counted as a pass)"
fi

# A machine with no digest tool must refuse, not install. PATH is stripped to a
# directory holding only what install.sh needs to get as far as the preflight.
FAKEBIN="${TMP}/nohash"
mkdir -p "$FAKEBIN"
for t in bash curl awk grep sed tr head cat mktemp mkdir rm mv env uname chmod printf tput seq paste; do
  p="$(command -v "$t" 2>/dev/null || true)"
  [ -n "$p" ] && ln -sf "$p" "${FAKEBIN}/${t}"
done
: > "${TMP}/nohash.out"
NOHASH_HOME="${TMP}/nohash-home"; NOHASH_BIN="${TMP}/nohash-bin"
mkdir -p "$NOHASH_HOME" "$NOHASH_BIN"
rc=0
env -i PATH="$FAKEBIN" HOME="$NOHASH_HOME" \
    WEDE_INSTALL_DIR="$NOHASH_BIN" \
    WEDE_INSTALL_BASE_URL="${BASE}/good" \
    WEDE_VERSION="$TAG" \
    "${BASH:-/bin/bash}" "$INSTALL_SH" > "${TMP}/nohash.out" 2>&1 || rc=$?
installed="no"; [ -e "${NOHASH_BIN}/wede${T_EXT}" ] && installed="yes"
diag="$(grep -m1 -E 'FATAL:' "${TMP}/nohash.out" | sed 's/.*FATAL: *//' || true)"
[ -n "$diag" ] || diag="(NO DIAGNOSTIC PRINTED)"
verdict="ok"
if [ "$rc" -eq 0 ] || [ "$installed" != "no" ] || ! grep -qF "no SHA-256 tool found" "${TMP}/nohash.out"; then
  verdict="FAIL"
  FAILURES=$((FAILURES + 1))
fi
N=$((N + 1))
printf '  %-32s exit %-4s installed=%-4s %-30s %s\n' \
  "no sha256 tool on PATH" "$rc" "$installed" "$verdict" "$diag"

printf '\n'
if [ "$FAILURES" -ne 0 ]; then
  printf '%s: %d of %d case(s) did not behave as specified.\n' "$SELF_NAME" "$FAILURES" "$N" >&2
  exit 1
fi
printf '%s%s: %d cases — install.sh installed bytes ONLY on the case where the published digest matched.%s\n' \
  "$GRN$BLD" "$SELF_NAME" "$N" "$RST"
