# Changelog

All notable changes to wede are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- **Command palette (Ctrl/Cmd+Shift+P)** — fully functional fuzzy-search command palette
  wired to all major IDE actions: New File, Save, Save All, Toggle Terminal, Open Settings,
  Focus Explorer, Focus Git, Open Browser Preview, Close Tab, Refresh Explorer, Git Stage
  All / Unstage All, Switch Theme, Log Out. Arrow-key navigation, Enter to run, Esc to close.
  Shortcut listed in the Settings panel shortcuts section.
- **Recursive directory copy** — `POST /api/files/copy` backend endpoint copies files and
  directories recursively under the same `safePath` workspace-confinement guard used by all
  other file endpoints. "Copy" is now re-enabled in directory context menus in the file
  explorer, and paste uses the new endpoint for both files and directories.
- **Ctrl/Cmd+W** global shortcut to close the active editor tab.

### Fixed
- **Old brand references removed** — all remaining mentions of the previous brand name
  (including the historical changelog entry) have been scrubbed from the codebase.

### Security
- **Server-side logout** — `DELETE /api/auth/logout` revokes the session token on disk;
  tokens are no longer valid after logout even if replayed from another client.
- **Session TTL** — session tokens now carry a `created_at` timestamp and expire after
  24 hours idle. Expired tokens are pruned from disk on the next login or auth check.
- **Lockout persistence** — brute-force attempt count and locked state are written to
  `~/.wede/lockout.json` so a server restart no longer resets the lockout counter.
  Unlock by deleting that file (instructions now printed in the error message).
- **Folder picker path escape** — `GET /api/workspace/browse` now rejects `?path=`
  values outside the user's home directory tree, preventing filesystem enumeration.
- **WS token moved out of URL** — terminal WebSocket now sends the auth token as a
  `auth.<token>` WebSocket subprotocol instead of a `?token=` query parameter.
  Access logs and browser history no longer contain the session secret.
- **Startup password redaction** — plaintext password is no longer logged at startup.

### Fixed
- **Config unknown keys are now fatal** — `wede.config.json` is decoded with
  `DisallowUnknownFields()`, so a typo like `"frame_ancestor"` (missing `s`) causes
  an immediate startup error rather than silently being ignored.
- **Delete confirmation** — right-click → Delete in the file explorer now shows a
  confirmation dialog before removing files or directories.
- **Directory Copy removed** — "Copy" is no longer shown in directory context menus;
  the file-read-based copy would silently fail on directories. File copy is unchanged.
- **Ctrl+V paste target** — keyboard paste now inserts into the last focused directory
  in the tree instead of always targeting the workspace root.
- **Dead "Command palette" shortcut removed** — Settings no longer advertises
  `Ctrl/Cmd+Shift+P` because no command palette is implemented.

### Changed
- **CI** — `ci.yml` now runs `go test ./...` (hard gate) and `npm run lint`
  (advisory — pre-existing JS violations tracked separately).
- **Config example** — `wede.config.example.json` added with placeholder password.
  `wede.config.json` is now gitignored to prevent committing real credentials.

### Removed
- **`database/` module** — the orphaned Postgres migration tool (`database/go.mod`,
  `migrate.go`, SQL migrations, env files) has been deleted. It was never referenced
  by the main binary and contradicts the "no database dependency" design goal.
- **`.env.dev` / `.env.main`** — environment files used only by the deleted database
  module have been removed.

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
- **Rebrand to Vulos / wede** — repository moved to `github.com/vul-os/wede`;
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
