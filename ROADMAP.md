# wede Roadmap

wede is a lightweight, self-hosted web IDE maintained by [Vulos](https://vulos.org).
The design goals are: single binary, no mandatory cloud, fast cold-start, and clean
embed-in-shell support. This roadmap tracks the public direction.

Items are grouped by milestone. Completed items move to [CHANGELOG.md](CHANGELOG.md).

---

## v0.2.x — Hardening (current)

- [x] Vulos OS `frame_ancestors` embed config
- [x] Security fixes: path-traversal, git arg-injection, WS origin, localhost bind
- [x] IDE redesign: Midnight/Daylight themes, responsive mobile layout
- [x] Visual git commit graph (DAG view)
- [x] `--version` flag + version injected via ldflags
- [x] `go test ./...` CI gate (`ci.yml` now runs Go tests)
- [x] `npm run lint` CI step (advisory — pre-existing violations tracked separately)
- [x] Config validation: unknown JSON keys are fatal on startup (`DisallowUnknownFields`)
- [x] Session expiry: 24 h idle TTL on session tokens
- [x] Server-side logout (`DELETE /api/auth/logout`) — tokens revoked on disk
- [x] Brute-force lockout persisted to disk — survives server restart
- [x] `HandleBrowse` path escape: folder picker confined to home directory tree
- [x] WS token no longer in URL — passed as `auth.<token>` subprotocol
- [x] Plaintext password removed from startup log
- [x] Delete confirmation dialog in file explorer (especially for directories)
- [x] Ctrl+V paste targets focused directory, not always workspace root
- [x] **Command palette implemented** — Ctrl/Cmd+Shift+P; fuzzy search; all IDE actions wired
- [x] **Recursive directory copy** — `POST /api/files/copy`; Copy/Paste re-enabled for both
  files and directories via the new recursive endpoint; safePath-guarded
- [x] **Ctrl/Cmd+W** close active tab shortcut
- [x] All legacy brand references removed from codebase, docs, and configs
- [x] Orphaned `database/` Postgres module deleted
- [x] `wede.config.json` gitignored; `wede.config.example.json` added

---

## v0.3.0 — Editor polish

- [x] **Multi-cursor** and column-select — `rectangularSelection` + `crosshairCursor`
  CodeMirror extensions; Alt+Click / Alt+Drag.
- [x] **Search across files** — ripgrep subprocess (Go walker fallback); Search sidebar
  panel; results grouped by file with highlighted matches; click to open at line.
- [ ] **File creation/deletion** keyboard shortcuts in file explorer
- [x] **Editor settings panel** — font size (10–24 px), tab width (2/4/8), word wrap,
  auto-save toggle; all settings live-applied via CodeMirror Compartments.
- [x] **Minimap** toggle — `@replit/codemirror-minimap` (MIT) wired via a CodeMirror
  `Compartment`; toggled in Settings; scroll-synced viewport overlay; live enable/disable
  without editor rebuild.
- [x] **Auto-save** — 1.5 s debounce; status indicator in top bar; toggleable in settings.
- [x] **Language Server Protocol (LSP) proxy** — Go backend (`backend/internal/lsp`)
  spawns one language server process per (workspace, language) pair and bridges
  JSON-RPC `Content-Length` frames ↔ WebSocket. Supported: `gopls` (Go),
  `typescript-language-server` (JS/TS), `pylsp` (Python), `rust-analyzer` (Rust).
  Client uses `codemirror-languageserver` (BSD-3) for diagnostics, hover, completion,
  and go-to-definition. Degrades gracefully when binary not installed — no errors,
  Settings panel shows which servers are active or missing. LSP toggled per-user in
  Settings. `GET /api/lsp/available` lists installed servers.
- [x] **Git push / pull / fetch / create-branch** — Remote tab in git panel; backend
  endpoints with injection-safe arg validation.
- [x] **File-watching SSE** — `GET /api/watch` (fsnotify + 250 ms debounce); explorer
  and git status refresh automatically on file-system changes; git-status poll relaxed
  from 10 s to 30 s.
- [x] **Git diff viewer** — inline unified diff for staged/unstaged files; click-to-expand
  per file row in the Changes tab.
- [x] **Discard file changes** — trash icon to restore a file to HEAD; injection-safe backend.
- [x] **Stash save/pop/list** — full stash workflow in the Changes tab.
- [x] **Commit detail diff** — click a commit in History to see files changed + full diff.
- [x] **Format on save** — `gofmt` / `prettier` / `black` via `POST /api/files/format`;
  toggled in Settings; also available as "Format Document" in the command palette.
- [x] **Go to line (`Ctrl+G`)** — floating line-jump overlay in the editor; command palette entry.

---

## v0.3.x — IDE-class gaps (current)

- [x] **Merge-conflict resolution** — conflicted files detected in git status (`UU`/`AA`/`DD`/`DU`/`UD`),
  shown in a "Conflicts" section; inline resolver shows each `<<<`/`===`/`>>>` region with
  Accept Current / Accept Incoming / Accept Both buttons; "Resolve & Stage" writes and stages the
  file. Backend: `GET /api/git/conflict`, `POST /api/git/conflict/resolve`.
- [x] **Remote management** — add and remove git remotes from the Remote tab; strict name
  validation (`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`). Backend: `POST /api/git/remotes/add`,
  `POST /api/git/remotes/remove`.
- [x] **Replace across files** — replace mode in the Search panel; preview shows amber-tinted
  replacements per match; "Replace All" applies atomically per file. 200-file / 10k-replacement
  cap. Backend: `GET /api/search/replace-preview`, `POST /api/search/replace`.
- [x] **Image/binary preview** — images (png/jpg/gif/svg/webp) rendered inline as `<img>` with
  a checkerboard transparency background; other binary files show a "binary file" notice with size.
  Backend: `Read` now returns `fileType:"image"` with base64 data URL, or `fileType:"binary"` + size.
- [x] **Per-hunk staging** — each `@@` hunk header in the diff view has a "+" button to stage just
  that hunk via `git apply --cached`; likewise "–" to unstage via `--reverse`. Backend:
  `POST /api/git/stage-hunk`.

---

## v0.4.0 — Terminal improvements

- [ ] **Persistent terminal sessions** — reconnect without losing the PTY on browser reload
- [ ] **Custom shell selection** — `shell` config key (`/bin/bash`, `zsh`, `fish`, …)
- [ ] **Terminal copy mode** — keyboard-driven selection (tmux-style)
- [ ] **Split panes** — horizontal / vertical terminal splits within a tab

---

## v0.5.0 — Remote & collaboration

- [ ] **SSH workspace** — open a remote directory over SSH (Go ssh client, no local agent
  required); all file/git/terminal operations tunnel through the SSH connection
- [ ] **Read-only share link** — time-limited token that grants read-only editor access
  (no terminal, no writes) for code review or pair sessions

---

## Future / exploratory

- **Plugin API** — register custom sidebar panels or editor commands from a WASM module
- **Vulos workspace sync** — optional cloud bookmark of open project + scroll position
  when running inside Vulos OS (uses the Vulos fabric sync layer, opt-in)
- **Container workspace** — open a path inside a running Docker/OCI container
- **Offline PWA** — service-worker cache so the UI loads instantly (already a single
  binary; this is about browser-side asset caching)
- **Theme editor** — live-edit and export Midnight/Daylight colour tokens as JSON

---

## Non-goals

- **Mandatory accounts or cloud** — wede will always run fully offline/standalone.
- **Database dependency** — the binary stays self-contained; no SQLite, Postgres, or Redis.
- **Extension marketplace** — wede is intentionally small; plugin API (above) is the
  extensibility story, not a marketplace.
