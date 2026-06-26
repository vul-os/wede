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
- [ ] Frontend test runner (vitest) + smoke test → now first-class in **Wave 10**
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

### Wave 2 — Identity & presence  ✅
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
- [x] Frontend collab client: `useCollab(roomId, token, username)` — opens the collab WS
      (?token= auth like useLSP), parses roster, throttled `setViewing(file,line)`; defensive.
- [x] Frontend: avatar roster (`PresenceRoster` in IDE top bar); IDE publishes active tab via
      `setViewing`
- [x] Per-file presence dots in FileExplorer (roster → `presenceMap` path→members; colored
      dots + viewer-name tooltip on each file row)
- [x] Tests: presence join/leave fan-out; roster correctness (4 presence tests)

### Wave 3 — Shared terminal  ✅ (2 polish items → Wave 7)
- [x] One PTY, N subscribers, **output fan-out** (`session.subs` set; `broadcast` snapshots
      under lock then writes outside it; dead subs pruned). Per-room already via `Room.Terminal()`.
- [x] **Multi-writer input** — any subscriber writes to the pty (serialized via `s.pmu`)
- [x] **Resize policy**: last-writer-wins on the shared pty (documented in HandleWS)
- [x] **Late-joiner scrollback replay** — every (re)joining subscriber gets the 64 KB buffer
- [x] Tests: subscriber set add/remove, ring-buffer tail+copy; race-clean (`go test -race`)
- [x] Frontend: terminal WS room-scoped (`/api/rooms/{id}/terminal`, legacy fallback) so
      users in a room share one PTY per session id; sessions reconcile is room-scoped too
- [ ] "shared • N viewers" indicator + "X is typing" (needs a terminal-WS control message
      for the viewer count) — deferred to Wave 7 polish

### Wave 4 — Collaborative editing (ygo)  ✅ implemented end-to-end; collab gated OFF pending live verification
- [x] Add `reearth/ygo` (v1.29.0, pure-Go); `internal/collabdoc` smoke tests prove the API
      (`crdt.New`/`GetText`/`Transact`/`Insert`/`ToString`) **and** an encode-update →
      apply-to-fresh-doc round-trip (the basis of server↔client sync). NOTE: ygo ships a
      `provider/websocket` server speaking y-protocols sync+awareness — candidate to reuse
      instead of hand-rolling the protocol.
- [x] `internal/collabdoc.DocStore`: server-authoritative `crdt.Doc` per open file (keyed by
      room-relative path), seed-on-open, `Text`/`Encode`/`ApplyUpdate`/`Close`; 6 unit tests
      incl. peer convergence. Wired into `Room.Docs()` (lazy) + torn down in `shutdown()`.
- [x] Sync + awareness WS mounted: `Room.DocServer()` = ygo `provider/websocket` Server
      (per room, `AllowedOrigins` from frameAncestors), route `GET /api/rooms/{id}/doc/{room...}`
      ({room...} = file's relative path). `collabdoc.DiskPersistence.LoadDoc` seeds the doc
      from disk (path-traversal-guarded); 5 adapter tests. `Shutdown(ctx)` on room close.
- [x] Edit → disk: `StoreUpdate` debounces (600ms) → `DocProvider.GetDoc` →
      `YText("content").ToString()` → atomic write (temp+rename, mkdir parents,
      traversal-guarded). `Stop()` final-flushes on room close. 6 write-back tests (race-clean).
- [ ] Reconcile EXTERNAL change: watcher detects on-disk edit (git checkout, terminal) →
      re-seed open doc as CRDT update (cursors survive), avoiding feedback loops with our own
      write-back — its own slice (deferred)
- [ ] Doc persistence under `~/.wede/rooms/{id}/docs/`; flush-on-last-disconnect
- [x] Path-encoding contract: room name = base64url(relative path); backend `decodeRoom`
      decodes (raw-path fallback) at LoadDoc + write-back; deps added (`yjs`,
      `y-codemirror.next`, `y-protocols`, `y-websocket`); build green. (+2 Go tests)
- [x] `src/hooks/useYDoc.js`: y-websocket `WebsocketProvider` → `/api/rooms/{id}/doc`,
      room=`b64urlPath(path)` (UTF-8-safe, matches Go RawURLEncoding), `params:{token}`,
      awareness user{name,color}; exposes `ytext`/`provider`/`awareness`; defensive + disposes
      on unmount/file change. (not integrated into Editor yet)
- [x] Integrate `yCollab` into `Editor.jsx`: when `collab.ytext`+awareness present, yCollab
      owns the doc (seeded from Y.Text, not the prop), remote cursors via awareness; Editor
      skips onChange (so IDE never auto-saves/marks-modified) + content-sync; Mod-s no-op.
      IDE calls `useYDoc` for the active text file, stable per-user color. **Gated OFF by
      default** (`editorSettings.collab ?? false`) pending live WS verification — see note.
- [ ] LIVE-VERIFY the doc WS round-trip (provider connect + sync + write-back) against a
      running server, then default collab ON (Settings toggle DONE in Wave 7 — users can opt in)
- [ ] Tests: two-client convergence; external-edit reconciliation; reconnect (needs live/integration harness)

### Wave 5 — VS Code parity (mostly polish on existing)  ✅ (2 items ⏭️ deferred with reasons)
- [x] Quick Open `Cmd+P` fuzzy file finder (`QuickOpen.jsx` + backend `files.Tree`
      `/api/files/tree` (+room-scoped), noise-skipping flat index; 2 Go tests). Cmd/Ctrl+P
      opens it; Enter/↑/↓ navigate.
- [x] Editor tabs + dirty indicators (`EditorTabs.jsx`: `tab.modified` dot, close button) —
      already present. (split editor: deferred)
- [x] Breadcrumbs path bar (`Breadcrumbs.jsx` — segmented file path above the editor,
      desktop layout, text files only; display-only)
- ⏭️ Problems/Diagnostics panel — DEFERRED: diagnostics live inside the
      `codemirror-languageserver` extension and aren't exposed to the app layer; a separate
      panel needs a parallel LSP client or a fork. Hover/definition/diagnostics already work
      inline in the editor.
- ⏭️ Symbol outline (`Cmd+Shift+O`) — DEFERRED: same reason (documentSymbol requests are
      internal to `codemirror-languageserver`).
- ⏭️ Snippets + configurable keybindings; sticky scroll — DEFERRED (nice-to-have polish)
- [x] Multi-cursor, minimap, format-on-save, go-to-line, image preview, command palette,
      search+replace — already shipped (verified in components; see "Shipped" section)
- [x] Markdown preview (`MarkdownPreview.jsx` via `marked`; Edit/Preview toggle on
      `.md`/`.markdown` tabs; scoped `.wede-markdown` styles in index.css)
- [ ] File create/delete keyboard shortcuts in explorer (context-menu + command palette
      already cover create/delete; raw keyboard shortcuts deferred)

### Wave 6 — Git graph, features & merge conflicts (extend existing)  ✅ (advanced items ⏭️ deferred)
- [x] Visual commit **graph (DAG)** with ref badges (HEAD/branch/origin) — already shipped
      (`GitPanel.jsx` graph view + `git.Log`)
- [x] Branch management UI — create (`CreateBranch`), checkout (`handleCheckout`), and now
      **delete** (`git.DeleteBranch` `/api/git/branch/delete` `-d`, +room-scoped; GitPanel
      Branches tab trash button w/ confirm; 2 Go tests incl. flag-name rejection)
- [x] Per-hunk staging (`git.StageHunk` via `git apply --cached`) — already shipped
- [x] Inline diff viewer + commit-detail diffs (`CommitDiff`) — already shipped
- [x] Merge-conflict resolver (Accept Current/Incoming/Both, resolve & stage,
      `ConflictRegions`/`ConflictResolve`) — already shipped
- [x] Stash (save/pop/list/drop), discard, push/pull/fetch, remote add/remove — already shipped
- ⏭️ Side-by-side diff, `git blame` gutter, 3-way merge view, cherry-pick/revert, stage-by-line
      — DEFERRED: meaningful git UX is already covered; these are advanced polish, lower ROI

### Wave 7 — UI/UX polish + deferred items  🚧
- [x] **Collab Settings toggle** — `Settings.jsx` "Collaborative editing" switch bound to
      `editorSettings.collab` (default false), back-filled in IDE's settings loader. Users can
      now opt into Wave 4 collab editing without editing localStorage.
- [ ] Terminal viewer-count ("shared • N") via a terminal-WS control message
- [ ] Migrate one component's fetches to `roomUrl(roomId, …)` (demonstrative; legacy works)
- ⏭️ External-disk-change doc re-seed — DEFERRED (feedback-loop risk; needs careful design)
- ⏭️ Broader design/a11y/toasts/virtualized-tree pass — DEFERRED (large; not blocking)

### Wave 8 — Docs & README  ⬜
- [ ] Rewrite `README.md` for collaborative, multi-project model
- [ ] Update `docs/ARCHITECTURE.md` (Rooms, ygo, presence, shared terminal)
- [ ] New `docs/COLLABORATION.md` (concepts, security model, limits)
- [ ] Refresh Playwright screenshots to show collaboration; changelog + version bump

### Wave 9 — Security & auth hardening  ⬜  (decided 2026-06-26; see security-auth-model memory)
Threat anchor: editor = shared terminal = shell on host. Editors are trusted; the only
contained tier is **viewer**. Owner password stays in `wede.config.json` (bootstrap/admin).
- [x] **Roles + share tokens (backend):** `Role` (viewer/editor/owner), `shareToken` store
      under `~/.wede/tokens.json` **hashed at rest** (SHA-256; raw returned once at mint).
      `MintToken`/`RedeemToken`/`RevokeToken`/`ListTokens`; owner-password login → owner role;
      `Role(token)` accessor; sessions carry role. **Constant-time** compare (`crypto/subtle`)
      for password + token hash. 7 token tests (mint/redeem/role, owner-not-mintable,
      wrong/empty/expired/revoked rejected, list omits hash, capabilities). HTTP endpoints +
      enforcement land in the next slices.
- [ ] **Authorization enforcement:** middleware/role gate so `viewer` is blocked (403) from
      terminal, file writes (create/write/delete/rename/copy/format), git mutations, and
      collab doc edits (read-only). Editors/owner: full. Tests per protected route.
- [ ] **Constant-time** password + token comparison (`crypto/subtle`) — replaces `!=`.
- [ ] **Secrets deny-list:** file/search APIs refuse to serve `~/.wede/**` even if `~` is the
      open workspace (protects the viewer tier); secrets never inside a workspace.
- [ ] **TLS posture:** warn loudly (or refuse) when bound to a non-loopback address without
      TLS — plaintext password/token on the wire is the main residual risk.
- [ ] **Lockout on token redemption** (extend existing brute-force lockout); keep WS origin
      checks + per-room `safePath` confinement (already tested).
- [ ] Frontend: owner "Share / Invite" panel (mint token + role → copy invite link; list +
      revoke); token-redeem flow → session; role surfaced in UI (viewer sees read-only).
- [ ] Tests: unauthorized → 401; viewer → 403 on terminal/write/git-mutate; revoked/expired
      token rejected; constant-time path; deny-list blocks `~/.wede` reads.

### Wave 11 — Unify projects & rooms (nameable)  ⬜  (UX gap found in local testing 2026-06-26)
"Project" (RoomSwitcher UI) == "Room" (backend) — one concept. Today the migration is
half-done: the switcher sets `activeRoomId` which only drives collab, while files/git/
terminal/search/watch still use legacy (default-room) routes, and "Open Folder" changes a
global workspace behind the switcher's back. Result: switching a project doesn't change the
content, and the project list doesn't update on folder change. Finish the migration:
- [ ] **Active room drives everything (frontend):** thread `roomsApi.activeRoomId` into
      FileExplorer, Editor/IDE file ops, GitPanel, Terminal (already), SearchPanel, and the
      file-watch SSE; route via `roomUrl(roomId, …)` (legacy fallback only when no room).
- [ ] **Unify "Open Folder" → "New / Open Project":** opening a folder creates or switches a
      room (named); the switcher refreshes immediately (fixes "doesn't update on folder change").
- [ ] **Rename + close projects:** backend rename endpoint (`POST /api/rooms/{id}/rename`
      {name}, +room-scoped) + inline rename + close in `RoomSwitcher`; Go test.
- [ ] **Switching a project** re-renders the tree/editor/git/terminal against the new room.
- [ ] Tests: rename endpoint; switching rooms changes the served root (integration);
      frontend: switcher reflects create/rename/folder-open.

### Wave 12 — UI/UX expansion: windowing, multi-root, VS Code-grade git, mobile  ⬜  (requested 2026-06-26 during local testing)
**Naming:** user-facing container = **Workspace** (recommended — it can hold multiple
folders; "room"/"project" read as single-folder). Keep "room" as the backend term; surface
"Workspace" in the UI. (Confirm with maintainer.)
- [ ] **Terminals as movable windows:** keep the default bottom dock, but add an OS-window
      mode — each terminal can float / drag-reposition / resize, with per-user placement;
      a focus/maximize mode where one terminal fills the page and the rest go to background;
      a top-bar toggle to show/hide terminals.
- [ ] **Resizable + collapsible sidebar:** drag the file-explorer/sidebar width; fully
      hide/collapse it and restore. (Persist per user.)
- [ ] **Comprehensive cross-project search:** search across the whole workspace (and all roots);
      richer results UI; jump-to-match.
- [ ] **Multi-root workspaces:** add multiple folders to one workspace/room — backend
      `Room.Roots[]` (files/git/search/tree span roots, path resolution per-root); UI to
      add/remove folders in the workspace switcher.
- [ ] **VS Code-grade git graph:** show ALL branches with lanes, refs/tags, author/date —
      overhaul GitPanel's graph to approximate the VS Code Git Graph extension.
- [ ] **Responsive + mobile-friendly:** every new surface (windowed terminals, resizable
      sidebar, graph, multi-root) degrades gracefully on small screens.
- NOTE: large wave; sequence AFTER Waves 9/11 integrate (shares IDE.jsx). Split into slices:
  sidebar resize/collapse → terminal windowing → multi-root backend → cross-project search →
  git graph overhaul → mobile pass. Keep `make check` green each slice.

### Wave 10 — Test coverage hardening  ⬜  (mandate: extensive testing, 2026-06-26)
Tests are the safety net the autonomous loop can't replace with runtime checks.
- [ ] **Frontend test runner:** add `vitest` + jsdom; `npm test` script; wire `vitest run`
      into `scripts/check.sh` so the gate covers frontend unit tests too.
- [ ] **Frontend unit tests:** pure helpers + hooks with mocked fetch/WebSocket —
      `b64urlPath` (round-trips Go RawURLEncoding), QuickOpen fuzzy `score`, `Breadcrumbs`,
      `MarkdownPreview` render, `useRooms`/`useAuth` (login/redeem, roster), `useCollab`/
      `useYDoc` (construct/teardown, defensive nulls), role-gated UI (viewer hides terminal/save).
- [ ] **Backend integration tests:** spin the mux (or per-handler `httptest`) and assert the
      room-scoped route map, the auth middleware + `RequireOwner`/`RequireEditor` gates across
      a representative route of each mutating class, token mint→redeem→revoke→expire end-to-end,
      and `~/.wede` deny-list enforcement.
- [ ] **Coverage pass:** `go test ./... -cover`; fill the thinnest packages; document any
      intentionally-untested surface (live WS/PTY) with a reason.

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
  `IDE`. Check green.
- 2026-06-26: Wave 2 slice 4 — frontend collab client. `useCollab` hook (opens collab WS via
  `?token=` like useLSP, parses roster, throttled `setViewing`, fully defensive);
  `PresenceRoster` avatars in the IDE top bar; IDE publishes the active tab via `setViewing`.
  Confirmed the auth middleware authenticates WS via `?token=` (not the subprotocol). Check
  green.
- 2026-06-26: Wave 2 slice 5 — per-file presence dots in FileExplorer (roster threaded down
  as `presenceMap`; colored dots + tooltips per file row). Tidied a stale eslint-disable;
  lint fully clean. **Wave 2 COMPLETE** — the app is now visibly collaborative (roster +
  who-views-what). Next: Wave 3 — shared terminal (terminal.Hub output fan-out to N subscribers).
- 2026-06-26: Wave 3 slice 1 — shared terminal backend. Converted a session from one active
  conn to a subscriber SET: pty output fans out to all (`broadcast` snapshots subs under the
  lock, writes outside it, prunes dead); every (re)joining viewer replays the 64KB scrollback;
  any viewer can type (pty writes serialized via `s.pmu`); resize is last-writer-wins. Added
  subscriber-set + ring-buffer tests; `go test -race` clean; full check green. Existing
  single-user terminal still works (one subscriber).
- 2026-06-26: Wave 3 slice 2 — terminal WS room-scoped on the frontend
  (`/api/rooms/{id}/terminal`, legacy fallback when room id unresolved); `roomId` threaded
  IDE→TerminalPanel→Terminal; sessions-reconcile room-scoped. Now users in a room actually
  share one PTY per `term-N` id. Minimal change — auth mechanism (auth.<token> subprotocol)
  untouched. **Wave 3 functionally COMPLETE**; viewer-count/"X is typing" indicator deferred
  to Wave 7 (needs a terminal-WS control message). Check green. Next: Wave 4 — ygo collab editing.
- 2026-06-26: Wave 4 slice 1 — added reearth/ygo v1.29.0 (network available; pure-Go, no
  cgo). New `internal/collabdoc` with 2 smoke tests: Doc/GetText/Transact/Insert/ToString,
  and EncodeStateAsUpdateV1 → ApplyUpdateV1 round-trip (fresh doc converges). go.mod at repo
  root. Discovered ygo's `provider/websocket` does the full y-protocols sync+awareness server
  — strong candidate to reuse for the collab WS doc channel. Check green.
- 2026-06-26: Wave 4 slice 2 — `collabdoc.DocStore`: server-authoritative `crdt.Doc` per open
  file (seed-on-open, Text/Encode/ApplyUpdate/Close, single-mutex serialized). 6 unit tests
  incl. encode→apply peer convergence. Wired `Room.Docs()` (lazy) + CloseAll on shutdown.
  Explored ygo `provider/websocket`: `Server` + `PersistenceAdapter{LoadDoc,StoreUpdate}` is
  the seam — LoadDoc seeds from disk, StoreUpdate persists; mount next slice. Check green.
- 2026-06-26: Wave 4 slice 3 — mounted ygo sync+awareness WS. `collabdoc.DiskPersistence`
  (LoadDoc reads file → seeds YText 'content' → EncodeStateAsUpdateV1; traversal-guarded;
  StoreUpdate no-op TODO); 5 tests. `Room.DocServer()` = `ywebsocket.NewServerWithPersistence`
  (AllowedOrigins from frameAncestors), `Shutdown(ctx)` on close. Route
  `GET /api/rooms/{id}/doc/{room...}` via Manager.Scoped (provider reads PathValue("room")).
  `go mod tidy` pulled provider transitive deps (x/sync, x/time, +indirect miniredis/gopher-lua).
  Check green.
- 2026-06-26: Wave 4 slice 4 — edit→disk write-back. `DiskPersistence.StoreUpdate` now
  debounces (600ms) and materializes the live doc text via a `DocProvider` interface
  (ygo `Server.GetDoc` → `YText('content').ToString()`) to an atomic temp+rename write
  (mkdir parents, traversal-guarded). `Stop()` final-flushes on room close (wired in
  `Room.shutdown` before `Server.Shutdown`). 6 write-back tests incl. subdir/new-file, Stop
  flush, traversal block; race-clean. External-disk-change re-seed deferred to its own slice
  (feedback-loop risk). Check green. Next: FRONTEND yjs + y-codemirror.next binding.
- 2026-06-26: Wave 4 frontend slice A — path-encoding contract settled: room name =
  base64url(relative path). Backend `decodeRoom` (RawURLEncoding, raw-path fallback) wired
  into LoadDoc + write-back; +2 Go tests. Added frontend deps yjs/y-codemirror.next/
  y-protocols/y-websocket; `npm run build` green (not integrated yet). Check green. Next:
  `useYDoc` hook (y-websocket provider + awareness) then `yCollab` in Editor.jsx.
- 2026-06-26: Wave 4 frontend slice B — `useYDoc.js`: opens a `Y.Doc` + y-websocket
  `WebsocketProvider` to `/api/rooms/{id}/doc/{b64urlPath(path)}?token=`, sets awareness
  user{name,color}, exposes `ytext`('content')/`provider`/`awareness`; disposes provider+doc
  on unmount/file change; fully defensive (nulls when inactive). `b64urlPath` is UTF-8-safe
  and matches Go RawURLEncoding. Not integrated into Editor yet. Check green. Next: yCollab
  in Editor.jsx.
- 2026-06-26: Wave 4 frontend slice C — yCollab integrated into Editor.jsx. When collab is
  active, yCollab owns the doc (seeded from Y.Text), remote cursors via awareness; Editor
  skips onChange/content-sync/Mod-s so IDE's REST auto-save never fights the CRDT write-back
  (tab never becomes "modified" → manual save also no-ops naturally). IDE calls `useYDoc` for
  the active text file. **Wave 4 is implemented end-to-end but collab is gated OFF by default**
  (`editorSettings.collab ?? false`) because the loop can't runtime-test the WS round-trip and
  a failed connect would hide on-disk content; opt-in via the setting. Non-collab editing is
  byte-for-byte unchanged. Live verification + default-on + Settings toggle deferred to Wave 7.
  Check green.
- 2026-06-26: Wave 5 slice 1 — Quick Open (Cmd/Ctrl+P fuzzy file finder). Backend
  `files.Tree` returns a flat, sorted, noise-skipped file index at `/api/files/tree`
  (+`/api/rooms/{id}/files/tree`), capped at 10k, 2 Go tests. Frontend `QuickOpen.jsx`
  modal: fuzzy filename ranking, keyboard nav, opens via `openFile`; wired into IDE
  (both layouts) + Cmd/Ctrl+P. Audited Wave 5: tabs+dirty indicators, multi-cursor,
  minimap, LSP hover/def/diagnostics, format-on-save, go-to-line, image preview, command
  palette, search+replace all already present (ticked). Remaining gaps: breadcrumbs,
  Problems panel, symbol outline, markdown preview. Check green.
- 2026-06-26: Wave 5 slice 2 — Breadcrumbs path bar (`Breadcrumbs.jsx`): segmented
  room-relative path shown above the editor in the desktop layout for text files;
  display-only, defensive (nulls → renders nothing). Check green.
- 2026-06-26: Wave 5 slice 3 — Markdown preview. `MarkdownPreview.jsx` renders via `marked`
  (gfm+breaks); Edit/Preview toggle bar on .md/.markdown tabs (works in both layouts via
  renderTabContent); scoped `.wede-markdown` styles restore headings/lists/code/tables that
  Tailwind's reset strips. Trust model noted (own-file render, no script eval). **Wave 5
  complete**; Problems panel + symbol outline + snippets/sticky-scroll DEFERRED (diagnostics/
  symbols are internal to codemirror-languageserver; inline diagnostics already work).
  Check green.
- 2026-06-26: Wave 6 — git audit + branch delete. Audited GitPanel.jsx + internal/git: DAG
  graph, diffs, per-hunk staging, merge-conflict resolver, stash, discard, push/pull/fetch,
  remotes, create/checkout branch all already shipped (ticked w/ evidence). Implemented the
  one genuine gap: branch DELETE — `git.DeleteBranch` (`-d`, force option, flag-name guarded;
  legacy + room-scoped routes; 2 Go tests) + GitPanel Branches-tab trash button (hover, confirm,
  refresh). **Wave 6 complete**; advanced items (side-by-side diff, blame, 3-way merge,
  cherry-pick/revert) deferred as lower-ROI polish. Check green.
- 2026-06-26: Wave 7 slice 1 — collab Settings toggle. `Settings.jsx` gains a "Collaborative
  editing" switch (bound to editorSettings.collab, default false, experimental helper text);
  `collab` back-filled in IDE's settings loader so it persists. Wave 4 collab editing is now
  user-enableable from the UI. Check green.
- 2026-06-26: Wave 9 slice 1 — share-token + roles backend. New roles.go (Role
  viewer/editor/owner, CanMutate, MintableRole) + tokens.go (hashed-at-rest share-token store
  in ~/.wede/tokens.json; Mint/Redeem/Revoke/List; raw token shown once). Sessions now carry
  Role; owner-password login → owner. Password + token comparisons are constant-time
  (crypto/subtle). 7 new token tests (12 auth tests total) green. HTTP mint/redeem endpoints
  + viewer authorization enforcement come next. Check green.
- 2026-06-26: **RENAME room -> workspace (no backwards compat), committed + pushed.**
  internal/room->internal/workspace (Room->Workspace); old internal/workspace->internal/folder
  (Workspace owns a folder.Manager); /api/rooms->/api/workspaces, /api/workspace->/api/folder;
  frontend useWorkspaces/WorkspaceSwitcher/workspaceUrl/workspaceId/workspacesApi/
  createWorkspace + all 'project'/'room' UI labels -> 'workspace'. ygo `{room...}` doc wildcard
  left as-is (ygo's term). Full check green; pushed to origin/feat/collab-ide (upstream set).
  WORKFLOW now: commit+push each green slice; NO backwards compat (remove legacy unscoped
  routes once frontend migrates in the unify wave).
