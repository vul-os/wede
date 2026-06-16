# Changelog

All notable changes to wede are documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- **Git diff viewer** — clicking any staged or unstaged file in the Changes tab expands an inline
  unified diff panel. Lines coloured green/red for additions/deletions; hunk headers muted.
  Truncates at 200 lines with a "N more lines" note. Uses `GET /api/git/diff?file=&staged=`.
- **Discard file changes** — trash icon on hover for unstaged file rows calls
  `POST /api/git/discard` (runs `git restore -- <path>`). Errors surface via a toast
  notification. Injection-safe: paths starting with `-` are rejected with 400.
- **Stash save / pop / list** — new "Stash" collapsible section at the bottom of the Changes tab.
  "Stash" button saves current changes; each stash entry shows a "Pop" button. Backend:
  `POST /api/git/stash`, `GET /api/git/stash`, `POST /api/git/stash/pop`, `POST /api/git/stash/drop`.
  Index validated as non-negative integer to prevent injection.
- **Commit detail diff** — left-clicking a commit row in the History graph opens a detail panel
  showing files changed and the full diff (`GET /api/git/commit-diff?hash=<hash>`). Hash
  validated to hex only (`^[0-9a-f]{4,64}$`). Per-file diff toggle included.
- **Format on save (gofmt / prettier / black)** — new "Format on save" toggle in Settings → Editor.
  When enabled, `Mod-s` pipes the current file content through the appropriate formatter before
  writing: `gofmt` for `.go`, `prettier` for JS/TS/CSS/JSON/HTML/MD, `black` for `.py`.
  Missing formatter binary → silently skips formatting, still saves. Bad syntax → skips
  formatting. Available as "Format Document" in the command palette. Backend endpoint:
  `POST /api/files/format`.
- **Go to line dialog** — `Ctrl+G` opens a floating "Go to line:" input overlay in the editor.
  Enter jumps and centres the view; Escape closes. Also available in the command palette.
  Wired via `onRegisterActions` ref to keep Editor.jsx self-contained.
- **Command palette entries** — new commands: "Go to Line…" (`Ctrl+G`), "Format Document".
- **Minimap** — real code minimap rendered by `@replit/codemirror-minimap` (MIT).
  Toggled via Settings → Editor → Minimap. Viewport overlay scroll-syncs with the
  editor; enable/disable is live (no editor rebuild via `Compartment`).
- **LSP proxy (diagnostics, hover, completion)** — Go backend package
  `backend/internal/lsp` spawns one language server process per (workspace, language)
  and bridges LSP JSON-RPC `Content-Length` frames to/from a WebSocket. Supported
  out of the box: `gopls` (Go), `typescript-language-server` (JS/TS), `pylsp`
  (Python), `rust-analyzer` (Rust). Binary discovery via `exec.LookPath` — no
  hard-coded paths. Frontend uses `codemirror-languageserver` (BSD-3-Clause) for
  diagnostics (squiggles), hover tooltips, completions, and go-to-definition.
  Degrades gracefully: if a binary is not installed, the WebSocket accepts and sends
  a single JSON-RPC notification then closes — no error UI, LSP features simply
  inactive. Settings panel shows which servers are active or hints to install them.
  LSP toggled per-user in Settings (persisted to `localStorage`). New endpoints:
  `GET /api/lsp` (WS), `GET /api/lsp/available`. Backend tests cover message
  framing, origin checks, and the availability table.
- **Command palette (Ctrl/Cmd+Shift+P)** — fully functional fuzzy-search command palette
  wired to all major IDE actions: New File, Save, Save All, Toggle Terminal, Open Settings,
  Focus Explorer, Focus Git, Open Browser Preview, Close Tab, Refresh Explorer, Git Stage
  All / Unstage All, Switch Theme, Log Out. Arrow-key navigation, Enter to run, Esc to close.
  Shortcut listed in the Settings panel shortcuts section.
- **Recursive directory copy** — `POST /api/files/copy` backend endpoint copies files and
  directories recursively under the same `safePath` workspace-confinement guard used by all
  other file endpoints. "Copy" is re-enabled in directory context menus; paste uses the new
  endpoint for both files and directories.
- **Ctrl/Cmd+W** global shortcut to close the active editor tab.
- **Auto-save** — debounced 1.5 s after the last keystroke; toggled per-user in Settings
  (default on). Status indicator ("saving…" / "saved") appears in the top bar while
  active. Manual Ctrl/Cmd+S still works regardless of the auto-save setting.
- **Project-wide search (Ctrl/Cmd+Shift+F)** — new Search sidebar panel with a 350 ms
  debounced query box, case-sensitivity toggle, and regex toggle. Backend uses ripgrep
  when on `$PATH` and falls back to a pure-Go `filepath.Walk` scanner. Results are
  grouped by file; clicking a match opens the file at the exact line. Workspace-confined
  via the same `safePath` guard used by all file endpoints. Skips `.git/`, `node_modules/`,
  and other build-artefact directories. Max 500 matches per query.
- **Git push / pull / fetch / create-branch** — new "Remote" tab in the git panel exposes
  fetch, pull, and push buttons with live output; backend endpoints validate remote and
  branch names to prevent flag-injection. Inline "New branch" input in the Branches tab
  creates and checks out a local branch via `git checkout -b`.
- **File-watching SSE** — `GET /api/watch` streams `text/event-stream` events using
  fsnotify. Explorer and git status refresh automatically on file-system changes
  (250 ms debounce); the 10 s git-status poll is relaxed to 30 s.
- **Editor settings** — Settings panel now has a dedicated Editor section: font size
  (10–24 px, +/− buttons), tab width (2/4/8 radio), word wrap toggle, auto-save toggle.
  All settings persist to `localStorage` under the key `wede_editor_settings` and take
  effect immediately without re-opening the editor.
- **Multi-cursor / column select** — CodeMirror's `rectangularSelection` +
  `crosshairCursor` extensions wired in. Alt+Click adds a cursor; Alt+Drag selects a
  rectangular region. Shortcut listed in the Settings shortcuts section.

### Fixed
- **Old brand references removed** — all remaining mentions of the previous brand name
  (including the historical changelog entry) have been scrubbed from the codebase.
- **Config unknown keys are now fatal** — `wede.config.json` is decoded with
  `DisallowUnknownFields()`, so a typo like `"frame_ancestor"` (missing `s`) causes
  an immediate startup error rather than silently being ignored.
- **Delete confirmation** — right-click → Delete in the file explorer now shows a
  confirmation dialog before removing files or directories.
- **Ctrl+V paste target** — keyboard paste now inserts into the last focused directory
  in the tree instead of always targeting the workspace root.

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

### Changed
- **CI** — `ci.yml` now runs `go test ./...` (hard gate) and `npm run lint`
  (advisory — pre-existing JS violations tracked separately, now resolved to 0 errors).
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
