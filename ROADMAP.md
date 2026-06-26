# wede Roadmap

wede is a lightweight, self-hosted web IDE maintained by [Vulos](https://vulos.org).
Historically single-user; the active direction is to make it a **multi-user,
multi-project collaborative IDE** that still ships as **one Go binary** — no cgo,
no Node sidecar, no external database.

This file is the durable source of truth. Checkbox rule: tick an item only when the
change **builds and tests pass** (`go build ./...`, `go test ./...`, `npm run build`,
`npm run lint`). Completed milestones are summarised under "Shipped". Honesty rule:
a wave is ✅ only when its acceptance tests pass — otherwise 🚧 with explicit TODOs.

**Latitude (granted 2026-06-26):** the maintainer is fine with the project being
*completely re-architected*. So the rebuild goes **room-native from the ground up** —
no obligation to preserve the old single-workspace API or thread backward-compat shims.
Two guardrails remain: (1) `make check` stays green **every cycle** so each commit is
usable; (2) "redone" means re-architected, **not feature-stripped** — carry the working
features (git graph, merge conflicts, LSP, editor, terminal) forward into the new shape.

**Branch:** `feat/collab-ide`

---

## North-star architecture (collaborative rebuild)

The core shift: **global singletons → per-`Room` state.** A `Room` *is* a project.
The server holds a `RoomManager` (`map[roomID]*Room`); every request is room-scoped,
so many people can work on many independent projects on one host at once.

```
Server
 └─ RoomManager  map[roomID]*Room
     └─ Room { Root, members,
               files, git, search,       // disk-backed, scoped to Root
               watcher (fsnotify),        // one per room
               lsp,                       // language servers per room (lazy)
               terms  (terminal.Hub),     // SHARED terminals, output fan-out
               docs   (collab.DocStore),  // ygo CRDT, server-authoritative
               presence }                 // roster, cursors, who-views-what
```

- **Collab editing:** [reearth/ygo](https://github.com/reearth/ygo) — pure-Go, cgo-free,
  Yjs-v13 wire-compatible CRDT. Server holds the authoritative `Y.Doc` per open file.
  Client: `yjs` + `y-codemirror.next`. Awareness layer = multiplayer cursors.
- **Disk⇄Doc reconciliation:** the per-room fsnotify watcher detects external edits
  (terminal `sed`, `git checkout`, formatters) and re-seeds the live doc. Central
  correctness problem; first-class treatment in Wave 4.
- **Routing:** path-scoped via Go 1.22+ `ServeMux` wildcards — `/api/rooms/{id}/...`.
  No router dependency.
- **Identity:** keep the shared-password gate as the door; add a **username at join**
  for presence/attribution. Per-user accounts stay optional/additive.
- **Lazy lifecycle:** watcher/LSP/PTYs start only when a room has ≥1 member and tear
  down (grace period) when empty — one binary hosts many projects without melting.

---

## Shipped (do not rebuild — audit & polish only)

These already exist in the codebase. Treat requests to "add/improve" them as
incremental enhancement on top of working features, not greenfield work.

- **Editor:** CodeMirror 6, multi-cursor + column select, minimap, settings panel
  (font/tab/wrap), auto-save (1.5 s debounce), go-to-line (Ctrl+G), format-on-save
  (`gofmt`/`prettier`/`black`), image/binary preview.
- **LSP:** Go backend proxy spawning one server per (workspace, language); `gopls`,
  `typescript-language-server`, `pylsp`, `rust-analyzer`; diagnostics, hover,
  completion, go-to-definition; degrades gracefully when a binary is missing.
- **Command palette:** Cmd/Ctrl+Shift+P, fuzzy, all actions wired.
- **Search:** ripgrep (Go-walker fallback) search **and replace** across files, with
  per-match preview and atomic per-file apply (200-file / 10k-replacement cap).
- **Git:** visual commit **graph (DAG)**, inline + commit-detail diffs, discard,
  stash save/pop/list, push/pull/fetch/branch, **remote add/remove**, **per-hunk
  staging** (`git apply --cached`), **merge-conflict resolver** (Accept Current /
  Incoming / Both, resolve & stage).
- **File explorer:** tree with git-status colors, context menu, recursive copy/paste,
  delete confirmation, paste-into-focused-dir, file-watch SSE auto-refresh.
- **Platform:** single binary, embedded frontend, shared-password auth, 24 h session
  TTL, disk-persisted brute-force lockout, WS auth via `auth.<token>` subprotocol,
  path-traversal / arg-injection hardening, Midnight/Daylight themes, responsive layout.

---

## Waves (collaborative direction)

Legend: ⬜ not started · 🚧 in progress · ✅ done (build+test green) · ⏭️ deferred

### Wave 0 — Safety net & scaffolding  🚧
- [x] Confirm baseline build/test/lint all green on `feat/collab-ide`
- [x] `Makefile` + `scripts/check.sh`: `build` / `test` / `lint` / `check`
- [x] Backend smoke test (`cmd/wede` securityHeaders) — establishes cmd test harness
- [ ] Frontend test runner (vitest) + one component smoke test
- [ ] Ensure `check` gates CI; document the dev loop in `docs/CONTRIBUTING.md`

### Wave 1 — Rooms backbone (the refactor)  ✅ (1 deferred-polish item → Wave 7)
No new user features — prove isolation.
- [x] `internal/room`: `Room` + `RoomManager`, lifecycle (create/get/list/close); each
      Room owns an isolated `workspace.Manager` (satisfies the WorkspaceProvider ifaces).
      Membership (join/leave) lands with Wave 2 presence.
- [x] `GET/POST /api/rooms`, `GET/DELETE /api/rooms/{id}` wired; boot workspace adopted
      as the "default" room (solo-user case, zero setup)
- [x] `internal/files` room-scoped — `Room.Files()` lazily binds a handler to the room's ws
- [x] `internal/git`, `internal/search` room-scoped (`Room.Git()`, `Room.Search()`)
- [x] `internal/filewatcher` one-per-room (`Room.Watcher()`, lazy) + `filewatcher.Close()`
- [x] `internal/terminal`, `internal/lsp` room-scoped (`Room.Terminal()`, `Room.LSP()`,
      lazy) + `Close()` on each; `frameAncestors` threaded via `NewManager(...)`
- [x] Path-scope routes: `/api/rooms/{id}/files|git|search|watch|terminal|lsp` (via `Manager.Scoped`)
- [x] Room-scoped `safePath` confinement — guaranteed by per-room workspace binding;
      proven by `TestCrossRoomConfinement` (room A 403s on `../roomB` traversal)
- [x] Teardown on close: `Room.shutdown()` closes the watcher via `Manager.Close`;
      lazy start-on-first-use done. Member-driven start + grace-period teardown pending (Wave 2)
- [x] Room-native API; auto-created boot room covers the solo case
- [x] Tests: two rooms / two roots / no cross-talk (room_test.go); lifecycle get/list/close
- [x] Frontend API foundation: `src/api.js` (`roomUrl`/`roomsUrl`) + `useRooms` hook
      (live: fetches `/api/rooms`, tracks active room, `createRoom`); wired into `App`,
      `roomId` threaded to `IDE`
- [x] Frontend: visible room switcher UI in IDE header (`RoomSwitcher.jsx` — list/switch/
      create projects via `roomsApi`)
- [ ] Migrate component fetches to `roomUrl(roomId, …)` (legacy default-room paths still
      work meanwhile) — deferred polish; tracked under Wave 7

### Wave 2 — Identity & presence  🚧
- [x] Username at join: `sessionEntry.Username` (backward-compatible), accepted on
      `POST /api/auth/login`, returned from `GET /api/auth/check`, updatable via
      `POST /api/auth/username`, `Username(token)` helper (5 auth tests). Frontend:
      Login display-name field → `useAuth` stores `wede_username` + exposes `username`,
      threaded to `IDE`. (`rooms[]` on session deferred — not needed yet)
- [x] `internal/presence`: per-room hub, roster, join/leave/update events (transport-agnostic
      outbound channels); wired into `Room.Presence()` + torn down in `shutdown()`
- [x] Single **collab WebSocket** `/api/rooms/{id}/collab` (`internal/collab`): auth-subprotocol
      upgrade, write-pump drains roster channel + pings, read-pump parses `{type:cursor}` →
      `Hub.Update`; `Room.Collab()` (lazy) + route wired. Doc/file events layer on later.
- [x] Broadcast "X is viewing `file`" + cursor line (`Hub.Update`); stable per-user color (palette)
- [ ] Frontend: avatar roster; per-file presence dots in FileExplorer
- [x] Tests: presence join/leave fan-out; roster correctness (4 presence tests)

### Wave 3 — Shared terminal  ⬜
- [ ] `terminal.Hub` per room: one PTY, N subscribers, output fan-out
- [ ] Multi-writer input + "X is typing"; optional soft driver-lock
- [ ] Resize policy (fixed / smallest-client-wins) + UI dims affordance
- [ ] Late-joiner scrollback replay (reuse 64 KB buffer)
- [ ] Tests: two clients same output; input interleaving; reconnect replay
- [ ] Frontend: shared indicator + participant list per terminal

### Wave 4 — Collaborative editing (ygo)  ⬜
- [ ] Add `reearth/ygo`; verify wire round-trip vs pinned `yjs`
- [ ] `internal/collab` `DocStore`: one server-authoritative `Y.Doc` per open file
- [ ] Sync handshake + awareness over collab WS
- [ ] Open → seed doc from disk; edit → observe `YText` → debounced write to disk
- [ ] Reconcile: watcher detects external change → re-seed as CRDT update (cursors survive) + UX
- [ ] Doc persistence under `~/.wede/rooms/{id}/docs/`; flush-on-last-disconnect
- [ ] Frontend: `y-codemirror.next`; remote cursors/selections with names
- [ ] Tests: two-client convergence; external-edit reconciliation; reconnect

### Wave 5 — VS Code parity (mostly polish on existing)  ⬜
- [ ] Quick Open `Cmd+P` fuzzy file finder
- [ ] Editor tabs + dirty indicators + overflow; split editor
- [ ] Breadcrumbs path bar
- [ ] Problems/Diagnostics panel from LSP; references/rename/hover surfaced in UI
- [ ] Symbol outline (`Cmd+Shift+O`) + workspace symbols
- [ ] Snippets + configurable keybindings; sticky scroll; bracket-pair colorization
- [ ] Markdown preview
- [ ] File create/delete keyboard shortcuts in explorer (carried from v0.3.0)

### Wave 6 — Git graph, features & merge conflicts (extend existing)  ⬜
- [ ] Graph polish: branch lanes, refs/tags rendering, performance on large histories
- [ ] Branch/tag management UI (create, checkout, delete, merge, rebase)
- [ ] Stage by line (extend per-hunk); side-by-side diff viewer
- [ ] `git blame` gutter + commit details
- [ ] Merge-conflict resolver: 3-way view, navigate-conflicts, beyond current inline mode
- [ ] Cherry-pick, revert; richer remote status

### Wave 7 — UI/UX polish  ⬜
- [ ] Design pass: spacing, type, color tokens, dark/light parity
- [ ] Keyboard nav + a11y (focus rings, ARIA, SR labels)
- [ ] Loading / empty / error states; toasts
- [ ] Responsive + persisted panel layout; virtualized file tree & large-file handling
- [ ] Collaboration onboarding (share-room flow)

### Wave 8 — Docs & README  ⬜
- [ ] Rewrite `README.md` for collaborative, multi-project model
- [ ] Update `docs/ARCHITECTURE.md` (Rooms, ygo, presence, shared terminal)
- [ ] New `docs/COLLABORATION.md` (concepts, security model, limits)
- [ ] Refresh Playwright screenshots to show collaboration; changelog + version bump

---

## Execution model

Autonomous loop, ~15-min self-wakeup, picking the next unchecked item in wave order.
Each cycle: implement a coherent slice → build/test/lint → commit → tick boxes here →
schedule next wakeup. Multiple agents used **within** a wave for non-overlapping files;
the Rooms refactor (Wave 1) stays single-track to keep builds green.

## Risk register
- **ygo maturity (~21★):** mitigated by the standard Yjs wire format — the client is
  unaffected if we later swap to a Node `y-websocket` sidecar or another impl.
- **Disk⇄Doc divergence:** central correctness risk; gated behind reconciliation tests.
- **Rooms refactor blast radius:** touches every backend package; done first, single-track,
  behind a default-room shim.
- **Resource exhaustion on one host:** lazy room lifecycle from the start.

## Later / exploratory
- SSH workspace (open a remote dir over SSH; ops tunnel through).
- Container workspace (open a path inside a running OCI container).
- Plugin API (WASM sidebar panels / editor commands).
- Offline PWA asset caching; theme editor.

## Non-goals
- **Mandatory user accounts** — collaboration uses the shared-password gate plus a
  chosen username; named per-user accounts remain optional, never required.
- **External database** — the binary stays self-contained. Collaboration state uses
  ygo's cgo-free filesystem adapter under `~/.wede/`, not Postgres/Redis/standalone SQLite.
- **Mandatory cloud** — wede always runs fully self-hosted/standalone.
- **Extension marketplace** — the plugin API is the extensibility story, not a marketplace.

## Progress log
- 2026-06-26: Roadmap redone for the collaborative direction; existing shipped features
  inventoried as audit-and-polish. Branch `feat/collab-ide`. Beginning Wave 0.
- 2026-06-26: Wave 0 safety net landed (check gate, Makefile, cmd smoke test). Latitude
  granted: complete re-architecture OK, green-per-cycle + no feature-strip.
- 2026-06-26: Wave 1 slice 1 — `internal/room` (Room + RoomManager) + `/api/rooms`
  endpoints; boot workspace adopted as default room. 4 room tests green; full check green.
- 2026-06-26: Wave 1 slice 2 — per-room services: `Room.Files()/Git()/Search()` lazy
  accessors + `Manager.Scoped` dispatch; full `/api/rooms/{id}/files|git|search` route set
  wired (legacy routes retained for the un-migrated frontend). 6 room tests green; check green.
- 2026-06-26: Wave 1 slice 3 — filewatcher per-room: `Room.Watcher()` (lazy) + new
  `filewatcher.Close()`; `Room.shutdown()`/`Manager.Close` tear it down. Legacy `/api/watch`
  + new `/api/rooms/{id}/watch` both route through room watchers; standalone watcher removed.
  7 room tests green; check green.
- 2026-06-26: Wave 1 slice 4 — terminal + lsp per-room: added `terminal.Close()` /
  `lsp.Close()` (extracted from their OnChange teardown); `Room.Terminal()`/`Room.LSP()`
  (lazy), `frameAncestors` threaded via `NewManager(fa)`; `Room.shutdown()` reaps PTYs +
  language servers. Legacy /api/terminal,/api/lsp + new room-scoped routes both flow
  through the default room; standalone term/lsp handlers removed. Check green. **Backend
  room-scoping complete.** Next: room-scoped safePath confinement, then Wave 1 frontend.
- 2026-06-26: Wave 1 slice 5 — room-scoped safePath confinement proven
  (`TestCrossRoomConfinement`). Frontend API foundation: `src/api.js` + `useRooms` hook
  wired live into `App`; `roomId` threaded to `IDE`. Check green.
- 2026-06-26: Wave 1 slice 6 — `RoomSwitcher.jsx` in the IDE top bar: lists/switches
  rooms and opens new projects via `roomsApi`. **Wave 1 complete** (call-site fetch
  migration deferred to Wave 7 polish; legacy default-room routes work). Check green.
- 2026-06-26: Wave 2 slice 1 — `internal/presence` hub (transport-agnostic: per-member
  outbound JSON channels, roster broadcast on join/leave/update, palette colors). Wired
  into `Room.Presence()` (lazy) + torn down in `shutdown()`. 4 hub tests green; check green.
- 2026-06-26: Wave 2 slice 2 — `internal/collab` WebSocket: auth-subprotocol upgrade
  (mirrors terminal), write-pump (roster + 30s ping) / read-pump (`parseCursor` →
  `Hub.Update`), prompt teardown via stop chan + `hub.Leave`. `Room.Collab()` (lazy, avoids
  mutex reentrancy) + `/api/rooms/{id}/collab` route. parseCursor table test; vet clean;
  check green.
- 2026-06-26: Wave 2 slice 3 — username-at-join. Backend: `sessionEntry.Username`
  (backward-compatible), login accepts + returns it, `/api/auth/check` echoes it,
  `POST /api/auth/username` + `Username(token)` helper; 5 auth tests. Frontend: Login
  display-name field, `useAuth` persists `wede_username` + exposes `username`, threaded to
  `IDE`. Check green. Next: `useCollab` hook (open `/api/rooms/{id}/collab`, send cursor,
  expose roster) + presence roster UI + per-file dots in FileExplorer.
