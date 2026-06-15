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
- [x] "Copy" removed from directory context menus (file copy only)
- [x] Ctrl+V paste targets focused directory, not always workspace root
- [x] Dead "Command palette" shortcut removed from Settings shortcuts list
- [x] **Command palette implemented** — Ctrl/Cmd+Shift+P; fuzzy search; all IDE actions wired
- [x] **Recursive directory copy** — `POST /api/files/copy`; re-enabled in file explorer
- [x] **Ctrl/Cmd+W** close active tab shortcut
- [x] All legacy brand references removed from codebase, docs, and configs
- [x] Orphaned `database/` Postgres module deleted
- [x] `wede.config.json` gitignored; `wede.config.example.json` added

---

## v0.3.0 — Editor polish

- [ ] **Multi-cursor** and column-select (CodeMirror extension)
- [ ] **Search across files** (ripgrep subprocess or pure-Go walker, results panel)
- [ ] **File creation/deletion** keyboard shortcuts in file explorer
- [ ] **Editor settings panel** — font size, tab width, word wrap, ligatures
- [ ] **Minimap** toggle (CodeMirror extension)
- [ ] **Auto-save** with configurable debounce delay
- [ ] **Language Server Protocol (LSP) proxy** — forward LSP JSON-RPC over the Go backend
  to a user-installed language server (gopls, pyright, etc.). Client side via
  `@codemirror/lang-*` LSP adapters.

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
