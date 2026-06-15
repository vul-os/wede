# Changelog

All notable changes to wede are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

---

## [0.2.0] — 2026-06-15

### Added
- **Vulos OS embed support** — new `frame_ancestors` config field. When set, wede emits
  `Content-Security-Policy: frame-ancestors <value>` instead of `X-Frame-Options: DENY`,
  allowing the Vulos OS shell to embed wede as an iframe app while keeping the standalone
  default fully locked down.
- **Visual git commit graph** — branch/merge history rendered as an SVG DAG in the git
  panel. Right-click context menus on commits (checkout, copy hash).
- **`--version` flag** — prints the injected build version and exits.
- Version is now logged at startup (`wede vX.Y.Z running on http://...`).

### Changed
- **Rebrand: webcrft → Vulos / wede** — repository moved to `github.com/vul-os/wede`;
  hosted at `wede.vulos.org`. All internal references updated.
- **IDE redesign** — overhauled UI with Midnight (dark) and Daylight (light) themes;
  Space Grotesk / Inter / JetBrains Mono font stack; responsive mobile layout; tabbed
  terminal panel.
- **Localhost-only default bind** — `host` now defaults to `127.0.0.1` (was `0.0.0.0`),
  preventing accidental public exposure on local installs.
- **`package.json` version** set to `0.2.0` (was placeholder `0.0.0`).
- **Go badge** in README corrected to `go 1.25+` (matches `go.mod`).

### Fixed
- **Path-traversal (security)** — `HasPrefix(full, ws)` replaced with
  `HasPrefix(full, ws + separator)`, closing a prefix-collision attack where
  `/workspace2/evil` was incorrectly accepted when workspace was `/workspace`.
- **Git arg-injection (security)** — commit count parameter validated as a positive
  integer; branch names checked to not start with `-`; `git add` now uses `--` separator
  so paths beginning with `-` cannot be mistaken for flags.
- **WebSocket origin validation (security)** — custom `checkOrigin()` replaces Gorilla's
  permissive default. Allows no-origin (non-browser), same-origin (derived from Host +
  `X-Forwarded-Proto`), and `frame_ancestors` allowlist. Rejects all other origins.

---

## [0.1.2] — 2024-12-xx

Initial public release under the `vul-os/wede` namespace.

### Added
- Single-binary Go + React web IDE (~10MB, embedded frontend).
- CodeMirror 6 editor with syntax highlighting for 12+ languages.
- Full PTY terminal via xterm.js over WebSocket (multiple tabs).
- File explorer with VS Code-style git status colours.
- Git client: status, staging, commit, branch management, diff viewer.
- Built-in browser preview tab.
- Password authentication with 3-attempt lockout.
- `install.sh` one-liner installer with auto-generated password and config.
- CI (`ci.yml`) and release (`release.yml`) pipelines; cross-compiled binaries for
  linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64.

---

[Unreleased]: https://github.com/vul-os/wede/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/vul-os/wede/compare/v0.1.2...v0.2.0
[0.1.2]: https://github.com/vul-os/wede/releases/tag/v0.1.2
