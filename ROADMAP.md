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
- [ ] `go test ./...` CI gate (add test step to `ci.yml`)
- [ ] `npm run lint` CI gate
- [ ] Config validation: surface unknown JSON keys as warnings on startup
- [ ] Session expiry: configurable idle timeout (default 24 h)

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
