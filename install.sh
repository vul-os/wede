#!/usr/bin/env bash
# =============================================================================
# wede installer — downloads the release binary for this platform and VERIFIES
# it against the release's own checksums.txt before putting it on your PATH.
#
# WHAT CHANGED AND WHY
# ────────────────────
# This script used to `curl -fsSL -o "$TMP_FILE" "$DOWNLOAD_URL"`, `chmod +x`
# it and move it into ~/.local/bin, having checked nothing. The release
# workflow published checksums.txt the whole time; nothing ever read it. A
# checksum file nobody checks is decoration — it documents what the bytes
# should have been while the installer runs whatever bytes arrived.
#
# Every path out of the verification block below is either "verified" or
# "abort". There is no --skip-verify, no warn-and-continue, and specifically no
# path where a missing or unreadable checksums.txt means "nothing to check".
# That case is the whole point: a 404 on the manifest is exactly when you know
# least, and reporting it as fine converts "I don't know" into "it's fine".
#
# For verifying an asset you downloaded by hand — with per-failure exit codes
# and a synthetic-origin failure matrix — see scripts/verify.sh in this repo.
# This installer is the same contract in the shape of a one-liner.
#
# FOUR DEFECTS DELIBERATELY NOT PRESENT
# ─────────────────────────────────────
#  1. NO FALL-OPEN. Every fetch failure aborts. Nothing degrades to an
#     unverified install because GitHub, a CDN or the network misbehaved.
#  2. NO SILENT DEATH AT A PIPELINE. The old latest-version lookup was
#       LATEST=$(curl ... | grep '"tag_name"' | sed ...)
#     under `set -euo pipefail`. When the API rate-limited (HTTP 403 — the
#     common case for an unauthenticated curl), pipefail made the assignment
#     fail and `set -e` killed the script THERE, with no message, so the
#     `[ -z "$LATEST" ]` guard written on the next line could never run. Every
#     lookup here ends in `|| true` and is then explicitly tested for
#     emptiness. A guard has to be reachable to be a guard.
#  3. NO `\n` INSIDE A `%s`. `die` takes one argument per line and prints each
#     with its own printf, message as an ARGUMENT and never as the format
#     string, so no filename or digest can be read as a format or an escape.
#  4. NO SUBSTRING / REGEX NAME MATCH. The asset name is compared by awk
#     against field 2 as a string. A substring grep treats the name as a regex
#     and would happily hand back the digest of "<asset>.sig" or of a
#     differently-punctuated asset.
#
# Overrides, for testing and for mirrors:
#   WEDE_REPO              owner/name        (default vul-os/wede)
#   WEDE_VERSION           tag, e.g. v0.3.0  (default: resolve "latest")
#   WEDE_INSTALL_BASE_URL  releases base URL (default: the GitHub repo's)
#   WEDE_INSTALL_DIR       where the binary lands
# =============================================================================
set -euo pipefail

SELF_NAME="${BASH_SOURCE[0]##*/}"
MANIFEST="checksums.txt"

if [ -t 2 ] && command -v tput >/dev/null 2>&1; then
  RED="$(tput setaf 1)"; GRN="$(tput setaf 2)"; BLD="$(tput bold)"; RST="$(tput sgr0)"
else
  RED=''; GRN=''; BLD=''; RST=''
fi

# One argument per output line; the message is a printf ARGUMENT rendered with
# %s, never the format string and never %b. Defect 3 lived exactly here.
die() {
  printf '%s%s: FATAL:%s %s\n' "$RED" "$SELF_NAME" "$RST" "$1" >&2
  shift
  local line
  for line in "$@"; do printf '        %s\n' "$line" >&2; done
  exit 1
}
info() { printf '%s%s:%s %s\n' "$GRN" "$SELF_NAME" "$RST" "$*"; }

REPO="${WEDE_REPO:-vul-os/wede}"
BASE_URL="${WEDE_INSTALL_BASE_URL:-https://github.com/${REPO}/releases}"

# A plaintext origin would deliver the binary AND the manifest over the same
# unauthenticated channel, so comparing one against the other would prove only
# that a single attacker was self-consistent. Loopback is allowed so the
# failure-matrix test can stand up a synthetic origin.
case "$BASE_URL" in
  https://*) ;;
  http://127.0.0.1:*|http://127.0.0.1/*|http://localhost:*|http://localhost/*) ;;
  *) die "refusing a plaintext, non-loopback release origin:" \
         "  $BASE_URL" \
         "The binary and its checksums would both arrive unauthenticated." \
         "Use an https:// URL." ;;
esac

command -v curl >/dev/null 2>&1 || die \
  "curl is required but not installed." \
  "  Ubuntu/Debian: sudo apt install curl" \
  "  macOS:         brew install curl" \
  "  Fedora:        sudo dnf install curl"

# sha256 of a file, on whichever tool this machine has. There is deliberately
# no fourth branch that skips: a machine that cannot hash cannot verify, and an
# unverified install is the thing this script exists to prevent.
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" 2>/dev/null | awk '{print $1}' || true
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -- "$1" 2>/dev/null | awk '{print $1}' || true
  elif command -v openssl >/dev/null 2>&1; then
    # Modern openssl prints "SHA2-256(file)= <hex>", older "SHA256(file)= <hex>".
    openssl dgst -sha256 "$1" 2>/dev/null | awk '{print $NF}' || true
  fi
}

command -v sha256sum >/dev/null 2>&1 ||
  command -v shasum >/dev/null 2>&1 ||
  command -v openssl >/dev/null 2>&1 ||
  die "no SHA-256 tool found (need sha256sum, shasum or openssl)." \
      "Refusing to install an unverified binary. Install GNU coreutils" \
      "(Linux) or use the shasum that ships with macOS, and re-run."

printf '%sInstalling wede...%s\n\n' "$BLD" "$RST"

# ── Platform ─────────────────────────────────────────────────────────────────
OS="$(uname -s)"
EXT=""
case "$OS" in
  Linux*)  OS="linux";   INSTALL_DIR_DEFAULT="${HOME}/.local/bin" ;;
  Darwin*) OS="darwin";  INSTALL_DIR_DEFAULT="${HOME}/.local/bin" ;;
  MINGW*|MSYS*|CYGWIN*)
    OS="windows"
    EXT=".exe"   # the release publishes wede-windows-amd64.exe; without this
                 # the installer asked for a name no release has ever had and
                 # died on a 404 that looked like "no such version".
    INSTALL_DIR_DEFAULT="${LOCALAPPDATA:-$HOME/AppData/Local}/wede"
    ;;
  *) die "unsupported operating system: $OS" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "unsupported architecture: $ARCH" ;;
esac

INSTALL_DIR="${WEDE_INSTALL_DIR:-$INSTALL_DIR_DEFAULT}"

info "OS:   $OS"
info "Arch: $ARCH"

# ── Resolve the release tag ──────────────────────────────────────────────────
# GitHub's /releases/latest redirects to /releases/tag/<tag>; ask curl where it
# landed rather than parse the API, which rate-limits unauthenticated callers
# and used to kill this script silently at a pipeline (defect 2).
TAG="${WEDE_VERSION:-}"
if [ -z "$TAG" ]; then
  resolved=""
  resolved="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "${BASE_URL}/latest" 2>/dev/null || true)"
  [ -n "$resolved" ] || die \
    "could not resolve the latest release from ${BASE_URL}/latest" \
    "GitHub was unreachable, rate-limited, or this repo has no releases." \
    "Refusing to guess a version. Pass one explicitly:" \
    "  WEDE_VERSION=v0.3.0 sh install.sh"
  TAG="${resolved##*/}"
  # A redirect that lands on the releases index (no releases yet) yields
  # "latest" or "releases" here, not a tag. Downloading from that would 404
  # later with a confusing message; say what actually happened instead.
  case "$TAG" in
    v[0-9]*) ;;
    *) die "no release tag could be read out of the redirect target:" \
           "  $resolved" \
           "That is what an empty releases page looks like. Refusing to" \
           "continue rather than fetch an asset from a made-up tag." ;;
  esac
fi
info "Version: $TAG"

ASSET="wede-${OS}-${ARCH}${EXT}"
DOWNLOAD="${BASE_URL}/download/${TAG}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/wede-install.XXXXXX")"
trap 'rm -rf -- "$TMP_DIR"' EXIT INT TERM

# ── Download ─────────────────────────────────────────────────────────────────
info "downloading ${DOWNLOAD}/${ASSET}"
rc=0
curl -fSL -o "${TMP_DIR}/${ASSET}" "${DOWNLOAD}/${ASSET}" || rc=$?
if [ "$rc" -eq 18 ]; then
  die "the download of ${ASSET} was TRUNCATED (curl exit 18)." \
      "  ${DOWNLOAD}/${ASSET}" \
      "The origin declared a Content-Length and then closed the connection" \
      "early. Nothing was installed. Retry; this is a transport failure, not" \
      "evidence of tampering."
fi
[ "$rc" -eq 0 ] || die \
  "download failed (curl exit ${rc}): ${DOWNLOAD}/${ASSET}" \
  "Check that ${TAG} exists and publishes an asset named ${ASSET} at" \
  "  ${BASE_URL}"

# ── Verify (FAIL CLOSED) ─────────────────────────────────────────────────────
info "downloading ${DOWNLOAD}/${MANIFEST}"
curl -fsSL -o "${TMP_DIR}/${MANIFEST}" "${DOWNLOAD}/${MANIFEST}" || die \
  "no ${MANIFEST} in release ${TAG} — refusing to install unverified bytes." \
  "  ${DOWNLOAD}/${MANIFEST}" \
  "Every wede release publishes ${MANIFEST} alongside the binaries (see" \
  ".github/workflows/release.yml), so its absence means the release is" \
  "incomplete or this download was intercepted. Neither is a condition to" \
  "shrug at, and there is no flag here that makes it one." \
  "Report it at https://github.com/${REPO}/issues"

[ -s "${TMP_DIR}/${MANIFEST}" ] || die \
  "${MANIFEST} is empty (0 bytes) — refusing to install unverified bytes." \
  "An empty manifest vouches for nothing while looking like a manifest."

# An HTML body is what a captive portal, a login wall or a CDN answering 200
# for a missing key looks like. Parsing it would find no digests while looking
# like it tried, so name the real problem.
if head -c 512 -- "${TMP_DIR}/${MANIFEST}" | LC_ALL=C grep -qiE '<(!doctype|html|head|body)\b'; then
  die "the origin returned an HTML page where ${MANIFEST} was expected." \
      "  ${DOWNLOAD}/${MANIFEST}" \
      "That is a captive portal, a login wall, or a CDN error page — not a" \
      "manifest. Nothing was installed."
fi

# Exact field-2 string compare. `sha256sum` marks binary mode with a leading
# '*' on the name, so tolerate both spellings and nothing else. A substring
# grep here would treat every '.' and '-' in the asset name as regex and could
# return the digest of a neighbouring asset (defect 4).
EXPECTED="$(awk -v want="$ASSET" '$2 == want || $2 == "*" want { print $1; exit }' \
  "${TMP_DIR}/${MANIFEST}" 2>/dev/null || true)"
[ -n "$EXPECTED" ] || die \
  "${MANIFEST} has no entry for ${ASSET} — refusing to install an asset the" \
  "release does not vouch for." \
  "  ${DOWNLOAD}/${MANIFEST}" \
  "Names are matched exactly: '${ASSET}' does not match '${ASSET}.sig'."

# A well-formed line is "<64 hex>  <name>". Anything else means the manifest is
# truncated or in a foreign format; feeding it into the comparison below would
# produce a mismatch diagnostic that sends the reader hunting for tampering.
if ! printf '%s' "$EXPECTED" | grep -Eqx '[0-9a-fA-F]{64}'; then
  die "the entry for ${ASSET} in ${MANIFEST} is not a 64-hex SHA-256 digest:" \
      "  ${EXPECTED}" \
      "The manifest is malformed or truncated. Nothing was installed."
fi

ACTUAL="$(sha256_of "${TMP_DIR}/${ASSET}")"
[ -n "$ACTUAL" ] || die \
  "could not compute a SHA-256 digest for ${ASSET}." \
  "Refusing to report an install as verified without a computed digest."

# Lowercase both sides before comparing; nothing else about the strings is
# normalised, so a mismatch is a mismatch.
EXPECTED="$(printf '%s' "$EXPECTED" | tr 'A-F' 'a-f')"
ACTUAL="$(printf '%s' "$ACTUAL" | tr 'A-F' 'a-f')"

if [ "$EXPECTED" != "$ACTUAL" ]; then
  rm -f -- "${TMP_DIR}/${ASSET}"
  die "CHECKSUM MISMATCH for ${ASSET} — these are not the published bytes." \
      "  expected: ${EXPECTED}" \
      "  actual:   ${ACTUAL}" \
      "  source:   ${DOWNLOAD}/${MANIFEST}" \
      "DO NOT run this file. The download has been deleted and nothing was" \
      "installed. Either the transfer corrupted it or the binary was" \
      "substituted."
fi
info "verified sha256 ${ACTUAL}"

# Optional, and honest about being optional: if the GitHub CLI is present, also
# check the sigstore build provenance GitHub attached at release time. It is
# never load-bearing — a machine without `gh` still gets the digest check — but
# when it IS checked the run says so, and when it is not the run says that too.
# A pass must never imply more than it checked.
#
# Deliberately NOT gated on WEDE_INSTALL_BASE_URL. Gating it there would skip
# provenance exactly when the download came from a mirror — the one case where
# a hostile origin controls both the binary and the manifest, and therefore the
# only case where the digest comparison proves nothing on its own. The
# attestation is over the bytes and is bound to this repo, so it verifies the
# same whatever host served them.
ATTESTED="not checked (install the gh CLI to check build provenance)"
if command -v gh >/dev/null 2>&1; then
  if gh attestation verify "${TMP_DIR}/${ASSET}" --repo "$REPO" >/dev/null 2>&1; then
    ATTESTED="VERIFIED"
    info "build provenance attestation: VERIFIED"
  else
    rm -f -- "${TMP_DIR}/${ASSET}"
    die "build provenance attestation FAILED for ${ASSET} (repo ${REPO})." \
        "The digest matched ${MANIFEST}, but no valid sigstore attestation" \
        "ties these bytes to a workflow run in that repository — so the" \
        "manifest itself is unvouched-for and a matching digest proves only" \
        "that the manifest and the binary agree with each other." \
        "Re-run 'gh attestation verify <file> --repo ${REPO}' for detail." \
        "Nothing was installed."
  fi
fi

# ── Install ──────────────────────────────────────────────────────────────────
chmod +x "${TMP_DIR}/${ASSET}"
mkdir -p -- "$INSTALL_DIR"
mv -f -- "${TMP_DIR}/${ASSET}" "${INSTALL_DIR}/wede${EXT}"
info "installed ${INSTALL_DIR}/wede${EXT}"

if ! printf '%s' "${PATH}" | tr ':' '\n' | grep -qx -- "$INSTALL_DIR"; then
  printf '\n  Warning: %s is not in your PATH.\n  Run this to add it:\n\n' "$INSTALL_DIR"
  # SC2016: the "$PATH" below is meant to reach the user's rc file literally,
  # not to expand here. That is the point of the suggested line.
  # shellcheck disable=SC2016
  case "$OS" in
    darwin)  printf '    echo '\''export PATH="%s:$PATH"'\'' >> ~/.zshrc && source ~/.zshrc\n' "$INSTALL_DIR" ;;
    linux)   printf '    echo '\''export PATH="%s:$PATH"'\'' >> ~/.bashrc && source ~/.bashrc\n' "$INSTALL_DIR" ;;
    windows) printf '    setx PATH "%%PATH%%;%s"\n' "$INSTALL_DIR" ;;
  esac
fi

# ── Default config ───────────────────────────────────────────────────────────
CONFIG_DIR="${HOME}/.config/wede"
CONFIG_FILE="${CONFIG_DIR}/wede.config.json"
mkdir -p -- "$CONFIG_DIR"

if [ -f "$CONFIG_FILE" ]; then
  printf '\n  Config already exists at %s — leaving it alone.\n' "$CONFIG_FILE"
else
  # `|| true` so pipefail cannot kill the script here; the length check below
  # is the actual guard. An empty password would otherwise be written into the
  # config as `"password": ""` — an admin console with no password, created by
  # the installer, silently.
  DEFAULT_PASSWORD="$(LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom 2>/dev/null | head -c 24 || true)"
  [ "${#DEFAULT_PASSWORD}" -eq 24 ] || die \
    "could not generate an admin password from /dev/urandom." \
    "Refusing to write ${CONFIG_FILE} with a blank or short password — that" \
    "would leave the admin console open. wede itself is installed at" \
    "${INSTALL_DIR}/wede${EXT}; write the config by hand and re-run."

  (umask 077; cat > "$CONFIG_FILE" <<CONF
{
  "password": "${DEFAULT_PASSWORD}",
  "port": "9090"
}
CONF
  )
  chmod 600 "$CONFIG_FILE"
  printf '\n  Config created at: %s (mode 600)\n' "$CONFIG_FILE"
  printf '  Admin password:    %s\n' "$DEFAULT_PASSWORD"
  printf '  Port:              9090\n'
fi

printf '\n%sDone.%s  wede %s — sha256 %s\n' "$BLD" "$RST" "$TAG" "$ACTUAL"
printf '  build provenance: %s\n\n' "$ATTESTED"
printf '  Quick start:\n    cd /path/to/your/project\n    wede .\n    open http://localhost:9090\n\n'
