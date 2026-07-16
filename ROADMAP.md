# wede Roadmap

wede is a self-hosted, collaborative web IDE maintained by [Vulos](https://vulos.org):
a single Go binary (embedded React frontend, no cgo, no Node sidecar, no external
database) that serves real-time multi-user editing, shared terminals, VS Code-grade
git tooling, and per-workspace chat.

## Status

**wede is deprioritized and not under active development.** The Vulos suite's
focus is now the OS and its owned apps (OS, Office, Files, Relay, llmux); wede
is not a first-party product going forward. It remains available and
self-hostable as-is, community-maintained, with no functionality removed and
no further roadmap work planned by the maintainer. See the status note in
[README.md](README.md).

Contributions are welcome — this file exists to be honest about what already
works (don't re-build it) and what's genuinely still open, for anyone picking
the project up.

---

## Shipped

The collaborative rebuild (rooms → workspaces, CRDT editing, presence, shared
terminals, chat, roles) is implemented end-to-end. Current feature set (see
[README.md](README.md#features) for the authoritative, user-facing list):

- **Collaboration** — multiplayer cursors and presence (CRDT-backed via
  pure-Go [reearth/ygo](https://github.com/reearth/ygo), Yjs-compatible),
  shared terminals (multi-subscriber PTY fan-out), per-workspace chat
  (public channel committed to `.wede/chat.md`, private channel
  auto-gitignored, live git-activity messages derived from `git log`), and
  owner-minted share links scoped to editor/viewer roles (hashed at rest,
  constant-time compare, listable/revocable).
- **Workspaces** — multiple independent projects on one host, switchable from
  the top bar; multi-root workspaces (several folders open together, VS Code
  style) with aggregate search/git/quick-open.
- **Editor** — CodeMirror 6, 25+ languages, multi-cursor/column-select,
  minimap, LSP (gopls, typescript-language-server, pylsp, rust-analyzer, and
  any other server via `~/.wede/lsp.json`), DAP (`dlv`, `debugpy`,
  extensible), format-on-save, go-to-line, image/binary preview, markdown
  preview, auto-save.
- **Git** — visual commit graph (DAG, branches/merges/refs), inline and
  commit-detail diffs, per-hunk staging, discard, stash, push/pull/fetch,
  branch create/delete, remote add/remove, merge-conflict resolver
  (symlink-safe path confinement), blame, cherry-pick/revert/reset.
- **Search** — project-wide search and replace (ripgrep with a pure-Go
  fallback), regex/case/word/glob toggles, per-match preview.
- **Extras** — built-in browser preview, Postman-style API client, tasks
  runner (`~/.wede/tasks.json`).
- **Platform / security** — single embedded binary; password auth with
  persisted brute-force lockout; session TTL + server-side logout;
  role-gated mutating routes (editor required for terminal/LSP/DAP/file
  writes/git mutations); WS origin checks; path-traversal and symlink-escape
  hardening across files/git/search; control-character sanitization on chat
  input; `THIRD-PARTY-NOTICES.txt` (served at `/licenses.txt`) covering every
  bundled Go module, npm package, and vendored site asset; locally vendored
  fonts (no runtime Google Fonts fetch); pinned Go toolchain kept current
  against `govulncheck`.
- **Public exposure** — one-click tunnel over your own sovereign
  [Vulos Relay](https://github.com/vul-os/vulos-relay) server (embedded
  agent, single outbound `wss://` connection, SSRF-guarded, no inbound ports
  or static IP, no third-party `frpc` dependency).
- **Testing** — Go backend tests (incl. `-race`), frontend unit tests
  (vitest), and Playwright browser E2E driving the actual production bundle
  in real chromium (boot guard across all gated surfaces + core IDE flows).
  All wired into CI.

---

## Later / exploratory

Genuinely unfinished or unstarted — low priority given the deprioritized
status, open to community contribution:

- **Problems/Diagnostics panel** and **symbol outline** (`Cmd+Shift+O`) —
  diagnostics/hover/completion already work inline in the editor via
  `codemirror-languageserver`, but a dedicated panel needs a parallel LSP
  client or a fork of that dependency.
- **Snippets, configurable keybindings, sticky scroll** — nice-to-have editor
  polish, not started.
- **Terminal viewer-count indicator** ("shared • N viewers" / "X is typing")
  — needs a small terminal-WS control message; shared terminals themselves
  already work.
- **External-disk-change reconciliation for collaborative docs** — if a file
  changes on disk (e.g. `git checkout`, an external editor) while a CRDT doc
  session is open, the live doc isn't currently re-seeded from disk. Needs
  careful design to avoid a feedback loop with wede's own write-back.
- **Plugin API** — a WASM sidebar-panel/editor-command extension point;
  explicitly not a VS Code `.vsix`-style marketplace (out of scope by
  design — see Non-goals).
- **SSH workspace** — open a remote directory over SSH, tunnelling ops
  through the connection.
- **Container workspace** — open a path inside a running OCI container.
- **Offline PWA asset caching**; **theme editor** (beyond the built-in
  Midnight/Daylight themes).

---

## Non-goals

- **Mandatory user accounts** — collaboration uses the shared-password gate
  plus a chosen display name; named per-user accounts remain optional, never
  required.
- **External database** — the binary stays self-contained; collaboration
  state lives under `~/.wede/`, not Postgres/Redis/standalone SQLite.
- **Mandatory cloud** — wede always runs fully self-hosted/standalone; the
  Vulos OS embed and Vulos Relay tunnel are opt-in, not required.
- **VS Code extension marketplace** — LSP/DAP configuration plus the planned
  WASM plugin API are the extensibility story, not a marketplace.

---

## Notes for contributors

- Keep `go build ./...`, `go test ./...`, `npm run build`, and `npm run lint`
  green on every change (`bash scripts/check.sh` runs the full gate).
- An **editor** share link grants an unsandboxed login shell on the host (see
  [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#no-sandbox-warning-for-shared-deployments));
  treat any change touching auth/roles/path-confinement as security-sensitive
  and add a regression test.
- See [CHANGELOG.md](CHANGELOG.md) for what has actually shipped, release by
  release.
