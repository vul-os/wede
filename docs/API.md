# HTTP & WebSocket API reference

wede's entire backend is `net/http` handlers wired into one `http.ServeMux` in
`backend/cmd/wede/main.go` — no framework, no generated client. This page
enumerates every route wired there as of the current source: **89 REST/SSE
routes** under `/api/`, **6 WebSocket endpoints**, and **1 SSE endpoint**
(counted within the 89). If you want the single source of truth instead of
this document, `main.go` is short enough to read end to end.

For the security model behind the role column (what an editor vs. a viewer
can actually do, and why the terminal is dangerous), see
[SECURITY-HARDENING.md](SECURITY-HARDENING.md) and
[ARCHITECTURE.md § 6](ARCHITECTURE.md).

## Conventions

- **Base URL**: everything below is relative to your wede instance, e.g.
  `http://127.0.0.1:9090`. All API routes are mounted under `/api/`.
- **JSON in, JSON out**: every non-WebSocket, non-SSE route sends and expects
  `application/json`, decoded/encoded with the standard library
  (`encoding/json`) — no envelope beyond what's shown per-route below.
- **Error shape**: handlers uniformly return `{"error": "<message>"}` with a
  non-2xx status (`400` bad input, `401` unauthenticated, `403` forbidden by
  role, `404` not found, `429` rate-limited, `500` server error). A few routes
  add extra fields on error — e.g. login's `{"error":"wrong_password",
  "remaining":2}` — noted inline where it matters.
- **Path parameters**: routes use Go's `net/http` `{id}` and `{room...}`
  wildcards (Go 1.22+ enhanced `ServeMux`), not a router library.
  - `{id}` is a **workspace ID** — an 8-byte-random hex string minted by
    `workspace.Manager` when a workspace is registered or created
    (`internal/workspace/workspace.go`, `newID()`). It is **not** a stable
    name: the boot workspace is registered with the display name `"default"`
    but still gets a random ID. Call `GET /api/workspaces` to discover the
    live ID(s) — don't hardcode `"default"`.
  - `{room...}` (CRDT doc socket only) is a base64url-encoded (no padding)
    workspace-relative file path.
- **Content-Type exceptions**: the file-watch endpoint is
  `text/event-stream` (SSE); the 6 WebSocket endpoints upgrade to `101
  Switching Protocols`; `GET /licenses.txt` is `text/plain`.

## Authentication

wede has exactly one owner (the config password) plus however many share
tokens the owner has minted. There are no user accounts.

### Getting a session

- **Owner login** — `POST /api/auth/login` with
  `{"password":"...", "username":"..."}` (username optional, cosmetic). On
  success (`200`): sets an `HttpOnly` cookie `wede_session` (`SameSite=Lax`,
  24 h `Max-Age`) **and** returns `{"token":"...", "username":"...",
  "role":"owner"}` in the body — a browser client typically relies on the
  cookie; a CLI/API client uses the returned token directly. Wrong password:
  `401 {"error":"wrong_password","remaining":N}`. After 3 wrong attempts:
  `403 {"error":"locked","message":"..."}`, persisted to
  `~/.wede/lockout.json` until that file is deleted.
- **Share-link redemption** — `POST /api/auth/redeem` (public, no auth
  required) with `{"token":"<raw invite token from the share link>"}`.
  Success (`200`): `{"token":"<new session token>", "role":"viewer"|"editor",
  "username":"..."}` — no cookie is set here; the frontend stores the
  returned token itself. Failure: `401 {"error":"invalid or expired
  token"}`. Rate-limited to **10 attempts per client IP per minute**
  (`429 {"error":"too many attempts; try again later"}`).
- **Checking session state** — `GET /api/auth/check` (public — it's how the
  frontend decides whether to show the login screen) returns
  `{"authenticated":bool, "locked":bool, "username":"...", "role":"..."}`.

### Presenting a session on subsequent requests

`auth.Middleware` (wraps every route under `/api/`) accepts the session token
from, in order:

1. `Authorization` header — raw token, no `Bearer` prefix.
2. `?token=` query parameter.
3. The `wede_session` cookie (browser login only).
4. For WebSocket upgrades only: an `auth.<token>` entry in the
   `Sec-WebSocket-Protocol` header (see below) — checked if none of the above
   yielded a token.

### WebSocket authentication

Browsers can't set arbitrary headers on a WebSocket handshake, so every WS
endpoint accepts the session token as a **`auth.<token>` WebSocket
subprotocol** — the client requests it via `Sec-WebSocket-Protocol:
auth.<token>`, and the server echoes the same subprotocol back on the `101`
response so the browser's handshake succeeds. This keeps the token out of
server access logs and browser history (unlike a `?token=` query string,
which `auth.Middleware` still accepts as a fallback for non-browser clients
such as `curl` or test tooling).

### Share-link redemption flow (end to end)

1. Owner calls `POST /api/auth/tokens` → gets back `{"raw": "<token>", "id":
   "...", "inviteUrl": "?invite=<token>"}` once.
2. Owner sends the resulting URL (`https://your-wede/?invite=<token>`) to the
   collaborator.
3. The frontend detects `?invite=` on load and calls
   `POST /api/auth/redeem` with that token, receiving a fresh session token
   scoped to the role the owner minted.

## Role requirement per route

Every session carries exactly one role, resolved server-side and injected
into the request context by `auth.Middleware` (`auth/roles.go`):

| Role | Obtained via | `CanMutate()` |
|---|---|---|
| **owner** | Password login | yes |
| **editor** | Redeemed editor share token | yes |
| **viewer** | Redeemed viewer share token | no |

Routes are gated one of three ways, and the tables below state which per
route:

- **owner** — wrapped in `auth.RequireOwner`; anything but an owner session
  gets `403`.
- **editor+** — wrapped in `auth.RequireEditor`; viewer sessions get `403`,
  owner and editor pass.
- **any** — only `auth.Middleware` applies (or, for a few, no auth at all —
  marked **public**); any authenticated role, including viewer, can call it.

## Auth & sessions

| Method & path | Role | Description |
|---|---|---|
| `POST /api/auth/login` | public | Owner password login. See above for body/response. |
| `GET /api/auth/check` | public | Current session status: `{authenticated, locked, username, role}`. |
| `POST /api/auth/redeem` | public | Exchange a raw share token for a session. Rate-limited 10/IP/min. |
| `DELETE /api/auth/logout` | any | Revokes the caller's session server-side; clears the `wede_session` cookie. `{"status":"ok"}`. |
| `POST /api/auth/username` | any | Sets the display username on the caller's session. Body `{"username":"..."}` → `{"username":"..."}`, or `401` if the session is invalid. |

## Share tokens (owner-only)

| Method & path | Role | Description |
|---|---|---|
| `POST /api/auth/tokens` | owner | Mint a share token. Body `{"role":"viewer"\|"editor","username":"...","ttlHours":0}` (`ttlHours` omitted/0 = never expires). Response `{"raw":"...","id":"...","inviteUrl":"?invite=..."}`. |
| `GET /api/auth/tokens` | owner | List live tokens (non-secret view): `{"tokens":[{id,role,username,createdAt,expiresAt}, ...]}`. |
| `DELETE /api/auth/tokens/{id}` | owner | Revoke a token by ID. `{"status":"ok"}` or `404`. |

## Workspaces

A **workspace** is one open project root. The boot path (CLI arg) is
auto-registered as a workspace named `"default"`; more can be opened at
runtime, each isolated behind its own `{id}`.

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces` | any | List open workspaces: `{"workspaces":[{id,name,root}, ...]}`. |
| `POST /api/workspaces` | editor+ | Open a new workspace. Body `{"name":"...","path":"..."}`; `path` is validated against `workspace_root` (see CONFIGURATION.md). `201` with `{id,name,root}`, or `400` on an invalid/disallowed path. |
| `GET /api/workspaces/{id}` | any | Get one workspace's `{id,name,root}`, or `404`. |
| `DELETE /api/workspaces/{id}` | editor+ | Close a workspace (tears down its terminal/LSP/watcher/chat/doc subsystems). `{"status":"closed"}` or `404`. |
| `GET /api/workspaces/{id}/wede-location` | owner | Which workspace-relative folder currently hosts `.wede/`: `{host, dir, folders}`. |
| `PUT /api/workspaces/{id}/wede-location` | owner | Relocate `.wede/` (moves its contents). Body `{"host":"subfolder"}`. Same response shape as GET, or `400`. |

## Folder picker (boot / default workspace)

These operate on the process's original boot workspace (`folder.Manager`),
distinct from the `{id}`-scoped workspace registry above.

| Method & path | Role | Description |
|---|---|---|
| `GET /api/folder` | any | `{current, recents, hasWorkspace}`. |
| `POST /api/folder/open` | editor+ | Body `{"path":"..."}`; validated against `workspace_root`. `{"status":"ok","current":"..."}` or `400`. |
| `GET /api/folder/browse` | any | Directory picker. Query `?path=`; confined to the home directory tree. Returns `{path, parent, dirs:[{name,path}], roots:[{name,path}]}`. |

## Files

All paths below are workspace-relative and confined to the workspace root by
`safePath()` (lexical + symlink-resolved check).

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/files` | any | List a directory. Query `?path=` (default: root). Returns `FileEntry[]` (`{name,path,isDir,size}`). |
| `GET /api/workspaces/{id}/files/tree` | any | Flat, sorted file-path list for Quick Open (skips `.git`/`node_modules`/hidden dirs, capped at 10,000 entries). `{"files":[...], "truncated":bool}`. |
| `GET /api/workspaces/{id}/files/read` | any | Read a file. Query `?path=`. Text files: `{"path","content"}`. Images (png/jpg/gif/svg/webp): `{"path","fileType":"image","dataUrl":"data:..."}`. Binary (null-byte heuristic) or >10 MiB: `{"fileType":"binary"|error, "size"}` without content. |
| `PUT /api/workspaces/{id}/files/write` | editor+ | Body `{"path","content"}` (request body capped 10 MiB). Creates parent dirs as needed. `{"status":"ok"}`. |
| `POST /api/workspaces/{id}/files/create` | editor+ | Body `{"path","isDir"}`. Creates an empty file or directory. |
| `DELETE /api/workspaces/{id}/files/delete` | editor+ | Query `?path=`. Recursive delete (`os.RemoveAll`); refuses to delete the workspace root itself. |
| `POST /api/workspaces/{id}/files/rename` | editor+ | Body `{"oldPath","newPath"}`. |
| `POST /api/workspaces/{id}/files/copy` | editor+ | Body `{"src","dst"}`. Recursive for directories. |
| `POST /api/workspaces/{id}/files/format` | editor+ | Body `{"path","content"}`. Runs a formatter (user-configured `.wede/formatters.json`/`~/.wede/formatters.json` first, else a built-in: `gofmt`, `prettier`, or `black` by extension) and returns `{"content","formatted":bool,"error":""}` — never a hard failure; `formatted:false` means the original content is returned unchanged. |

## File watching (SSE)

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/watch` | any | `text/event-stream`. One `fsnotify` watcher per workspace, 250 ms debounce. Events: `data: {"type":"ping"}\n\n` immediately on connect and every 15 s; `data: {"type":"change"}\n\n` when files change (no path detail — client should re-fetch). |

## Git

Read-only routes need no special role; every mutating route is `editor+`.
Argument validation (branch-name/`--` separators, hex-only hashes) guards
against flag injection into the `git` subprocess — see ARCHITECTURE.md § 6.

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/git/status` | any | `{branch, files:[{path,status,staged,conflicted}], isRepo}`. |
| `GET /api/workspaces/{id}/git/log` | any | Query `?count=` (default 50, clamped 1–10000). `{entries:[{hash,short,message,author,date,refs,parents,dateISO}]}`. |
| `GET /api/workspaces/{id}/git/diff` | any | Query `?file=&staged=true\|false`. `{"diff":"...unified diff..."}`. |
| `POST /api/workspaces/{id}/git/stage` | editor+ | Body `{"path":"..."}` (default `"."` = all). |
| `POST /api/workspaces/{id}/git/unstage` | editor+ | Body `{"path":"..."}` (default `"."`). |
| `POST /api/workspaces/{id}/git/commit` | editor+ | Body `{"message":"..."}` (required). `{"status":"ok","output":"..."}`. |
| `GET /api/workspaces/{id}/git/branches` | any | `{branches:[{name,current}], current}`. |
| `POST /api/workspaces/{id}/git/checkout` | editor+ | Body `{"branch":"..."}`. Rejects names starting with `-`. |
| `POST /api/workspaces/{id}/git/branch` | editor+ | Create a branch. Body `{"name","checkout":bool}`. |
| `POST /api/workspaces/{id}/git/branch/delete` | editor+ | Body `{"name","force":bool}` (`force` → `git branch -D`). |
| `POST /api/workspaces/{id}/git/fetch` | editor+ | Body `{"remote":"..."}` (optional; runs `git fetch --prune [remote]`). |
| `POST /api/workspaces/{id}/git/pull` | editor+ | Body `{"remote","branch"}` (both optional). |
| `POST /api/workspaces/{id}/git/push` | editor+ | Body `{"remote","branch","setUpstream":bool}`. |
| `GET /api/workspaces/{id}/git/remotes` | any | `{remotes:[{name,url}]}`. |
| `POST /api/workspaces/{id}/git/discard` | editor+ | Body `{"path":"..."}`. Restores a tracked file to HEAD (`git restore`); fails for untracked files. |
| `GET /api/workspaces/{id}/git/stash` | any | List stash entries: `{"stashes":[{index,message,date}]}`. |
| `POST /api/workspaces/{id}/git/stash` | editor+ | Body `{"message":"..."}` (optional). Creates a stash entry. |
| `POST /api/workspaces/{id}/git/stash/pop` | editor+ | Body `{"index":0}`. |
| `POST /api/workspaces/{id}/git/stash/drop` | editor+ | Body `{"index":0}`. |
| `GET /api/workspaces/{id}/git/commit-diff` | any | Query `?hash=` (must match a hex-commit-hash pattern). `{"stat","diff","files":["path", ...]}`. |
| `GET /api/workspaces/{id}/git/conflict` | any | Query `?file=`. `{"regions":[{index,currentLines,incomingLines,startLine,endLine}]}` — one entry per `<<<<<<< / ======= / >>>>>>>` block. |
| `POST /api/workspaces/{id}/git/conflict/resolve` | editor+ | Body `{"path":"...","resolutions":[{"index":0,"choice":"current"\|"incoming"\|"both"}]}`. Applies resolutions and re-stages the file. |
| `POST /api/workspaces/{id}/git/remotes/add` | editor+ | Body `{"name","url"}`. |
| `POST /api/workspaces/{id}/git/remotes/remove` | editor+ | Body `{"name"}`. |
| `POST /api/workspaces/{id}/git/stage-hunk` | editor+ | Body `{"patch":"...unified diff..."}`. Applies to the index via `git apply --cached`. |
| `GET /api/workspaces/{id}/git/blame` | any | Query `?file=`. Per-line blame attribution. |
| `GET /api/workspaces/{id}/git/tags` | any | All tags, most-recent creator date first. |
| `POST /api/workspaces/{id}/git/cherry-pick` | editor+ | Body `{"hash":"..."}`. |
| `POST /api/workspaces/{id}/git/revert` | editor+ | Body `{"hash":"..."}`. Creates a new commit undoing the given one. |
| `POST /api/workspaces/{id}/git/reset` | editor+ | Body `{"hash":"...","mode":"soft"\|"mixed"\|"hard"}`. |
| `POST /api/workspaces/{id}/git/merge` | editor+ | Body `{"branch":"..."}`. |
| `POST /api/workspaces/{id}/git/tag` | editor+ | Body `{"name","hash","message"}` (`message` present → annotated tag, else lightweight). |
| `POST /api/workspaces/{id}/git/tag/delete` | editor+ | Body `{"name"}`. |

## Terminals (WebSocket + listing)

See the [WebSocket protocol](#websocket-protocol) section for the wire
format. The terminal is a **shared, unsandboxed login shell** — read
[SECURITY-HARDENING.md](SECURITY-HARDENING.md) before granting editor access.

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/terminal/sessions` | any | List active terminal session IDs for this workspace: `{"sessions":["..."]}`. |
| `GET /api/workspaces/{id}/terminal` | editor+ | WebSocket. Query `?session=<id>` selects/creates a shared PTY session; omitted falls back to deriving the session ID from the auth subprotocol's token value. |

## Collaboration / CRDT editing (WebSocket)

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/doc/{room...}` | editor+ | CRDT document sync socket (one Yjs-compatible document per open file). `{room...}` is the base64url-encoded workspace-relative file path. Gated to editor+ because it drives writes back to disk. |

## Presence (WebSocket)

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/collab` | any | Presence roster + live cursor/mouse/window relay. Not editor-gated — viewers should still see who else is present. |

## Chat

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/chat` | any | WebSocket. Query `?channel=public\|private` (default public if omitted/unrecognized), `?color=` (cosmetic hex color). Public persists to `.wede/chat.md` (committed) plus live git-activity notices; private persists to `.wede/private/chat.md` (gitignored). Deliberately **not** editor-gated: viewers may read and post. |

## Search

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/search` | any | Content search. Query `?q=` (required), `?case=true`, `?word=true`, `?regex=true`, `?include=<glob>`, `?exclude=<glob>`, `?context=<0-5>`. Uses `rg` if on `PATH`, else a built-in walk. Response `{matches, truncated, count, fileCount}`, capped at 500 matches. |
| `GET /api/workspaces/{id}/search/files` | any | Filename search. Query `?q=` (required), `?case=`, `?regex=`, `?include=`, `?exclude=`. Response `{"files":[{"path"}], "count", "truncated"}`. |
| `GET /api/workspaces/{id}/search/replace-preview` | any | Preview a find/replace. Query `?q=&replace=&case=&word=&regex=&include=&exclude=`. Response `{matches: [{...Match, replacedText}], truncated, count, affectedFiles}`. |
| `POST /api/workspaces/{id}/search/replace` | editor+ | Apply a find/replace across matched files. Body `{"query","replace","caseSensitive":bool,"wholeWord":bool,"useRegex":bool,"includeGlob","excludeGlob","paths":[]}` (`paths` non-empty restricts to those files; `query` is required). |

## LSP (WebSocket)

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/lsp/available` | any | `{"available": {lang: binaryPath}, "extensions": {ext: lang}}` — which language servers are installed. |
| `GET /api/workspaces/{id}/lsp` | editor+ | WebSocket. Query `?lang=<language>` (required). One process per (workspace, language), shared across connections. |

## DAP (WebSocket)

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/dap/available` | any | `{"available": {lang: binaryPath}, "extensions": {ext: lang}}` — which debug adapters are installed. |
| `GET /api/workspaces/{id}/dap` | editor+ | WebSocket. Query `?lang=<language>` (required). A fresh adapter process per connection (no sharing, unlike LSP). |

## Tasks

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/tasks` | any | `{"tasks":[{name,command,cwd}]}` — the owner's global `~/.wede/tasks.json`, plus the workspace's committed `.wede/tasks.json` **only if the workspace is trusted**. Tasks execute client-side in a terminal (editor-gated there); this route only lists them. |

## Formatters

There is no dedicated formatters route — formatting happens through
`POST /api/workspaces/{id}/files/format` (above), which consults
`~/.wede/formatters.json` and, for trusted workspaces,
`<root>/.wede/formatters.json` before falling back to built-ins.

## Workspace trust (owner-only)

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/trust` | owner | `{"trusted":bool, "hasProjectConfig":bool}` — whether this workspace root is trusted, and whether it even ships a `.wede/{lsp,formatters,tasks}.json` worth trusting. |
| `POST /api/workspaces/{id}/trust` | owner | Marks the root trusted. `{"trusted":true}`. |
| `DELETE /api/workspaces/{id}/trust` | owner | Revokes trust. `{"trusted":false}`. |

## API client (built-in HTTP request runner)

Requests/environments persist as JSON files under `<wede>/requests/` and
`<wede>/environments/`. See [SECURITY-HARDENING.md § 10](SECURITY-HARDENING.md)
for the SSRF guard on `/send`.

| Method & path | Role | Description |
|---|---|---|
| `GET /api/workspaces/{id}/apiclient` | any | `{"tree": [...folders/requests...], "environments": [{name, variables}]}`. |
| `POST /api/workspaces/{id}/apiclient/send` | editor+ | Body `{"method","url","headers":{},"body"}`. Executes the request server-side. Response `{"status","statusText","headers","body","timeMs","size"}`, or `{"error","timeMs"}` (still HTTP `200`) on a transport failure. Blocked by default from reaching loopback/private/link-local/metadata targets. |
| `PUT /api/workspaces/{id}/apiclient/item` | editor+ | Body `{"type":"folder"\|"request","path","request"}`. Creates a folder or writes a request file. |
| `DELETE /api/workspaces/{id}/apiclient/item` | editor+ | Query `?path=&type=`. Deletes a request or folder (recursive). |
| `PUT /api/workspaces/{id}/apiclient/environment` | editor+ | Body `{"name","variables":{}}`. |
| `DELETE /api/workspaces/{id}/apiclient/environment` | editor+ | Query `?name=`. |

## Tunnel (public exposure, owner-only)

| Method & path | Role | Description |
|---|---|---|
| `GET /api/tunnel` | owner | `{"status","publicUrl","config":{serverUrl,token:"",name},"log":[...]}` — token always redacted. |
| `PUT /api/tunnel/config` | owner | Body `{"serverUrl","token","name"}`. An empty `token` preserves the currently stored one. Restarts the tunnel if it was running. |
| `POST /api/tunnel/start` | owner | Starts the tunnel against the stored config. |
| `POST /api/tunnel/stop` | owner | Stops the tunnel. |

## Misc / meta

| Method & path | Role | Description |
|---|---|---|
| `GET /licenses.txt` | public | Third-party license notices for bundled dependencies, generated by `scripts/gen-notices.sh`. Served before the auth gate. |
| `GET /login` | public | Always serves the SPA (shows the login form when logged out), regardless of `serve_landing`. |
| `GET /` | public | Authenticated request → the IDE SPA. Unauthenticated request → the marketing landing page if `serve_landing` is on (cloud-only), otherwise the SPA's login form. |
| `/site/*` | public | Standalone marketing site + docs viewer, only mounted when `serve_landing` is `true` (cloud deployments; a self-hosted instance leaves this off). |

---

## WebSocket protocol

All 6 WebSocket endpoints share the same auth mechanism (`auth.<token>`
subprotocol, see [Authentication](#authentication)) and the same
origin-check policy: same-origin or no `Origin` header is always allowed;
any other origin must appear in the space-separated `frame_ancestors` config
value, or the upgrade is rejected. Each endpoint's own message framing:

### Terminal — `GET /api/workspaces/{id}/terminal?session=<id>`

- **Server → client**: binary frames carrying raw PTY output, broadcast to
  every subscriber attached to the same session (a session is a *shared*
  terminal — multiple viewers/editors can attach to one PTY at once). On
  (re)connect, up to 64 KiB of scrollback is replayed as one binary frame
  before live output resumes.
- **Client → server**: a text frame containing `{"type":"resize","cols":N,
  "rows":N}` resizes the PTY (last writer wins across all attached clients).
  Any other message (text or binary) is written verbatim to the PTY's stdin.
- **Keepalive**: server sends a `PingMessage` every 25 s; read deadline is
  60 s, extended on every pong.
- **Lifecycle**: the shell is `$SHELL -l` (or `/bin/bash -l` /`cmd.exe` if
  `$SHELL` is unset), started in the workspace root, and persists across
  reconnects until the workspace changes (kills all sessions) or the shell
  process exits on its own.

### LSP — `GET /api/workspaces/{id}/lsp?lang=<language>`

- **Both directions**: text frames carrying a raw LSP JSON-RPC message body
  (no `Content-Length` header over the wire — that framing is added/stripped
  internally only between wede and the spawned language-server process's
  stdio).
- If `lang` isn't a known/configured language, or the binary isn't found on
  `PATH`, the server sends one `window/showMessage` JSON-RPC notification as
  a text frame, then closes — never an HTTP error (the upgrade already
  succeeded).
- One server process is shared per (workspace, language) across all
  connections; it's killed when the workspace changes.

### DAP — `GET /api/workspaces/{id}/dap?lang=<language>`

- Same framing convention as LSP (raw Debug Adapter Protocol JSON per text
  frame, `Content-Length` framing only between wede and the adapter's
  stdio).
- No process sharing — every connection gets and kills its own adapter
  process.
- If the adapter isn't available, the server sends a DAP `output` event
  (`{"type":"event","event":"output","body":{"category":"console",
  "output":"..."}}`) then the socket ends.

### Collab / presence — `GET /api/workspaces/{id}/collab`

- **Server → client**: `{"type":"presence","members":[{id,username,color,
  file,line}, ...]}` — the full roster, re-sent on every join, leave, or
  cursor update.
- **Client → server cursor update**: `{"type":"cursor","file":"...",
  "line":N}`.
- **Ephemeral relay**: a client may send `{"type":"mouse"|"window"|
  "terminals", ...}`; the server tags it with the sender's roster ID and
  relays it verbatim to every *other* connected member (not the sender,
  not persisted, not part of the roster snapshot).
- Identity (`username`) comes from the authenticated session, never a
  client-supplied field, so presence can't be spoofed. Ping every 30 s.

### Chat — `GET /api/workspaces/{id}/chat?channel=public|private&color=#hex`

- **Server → client**: `{"type":"history","messages":[Message, ...]}` once,
  immediately after connecting (merges persisted history with recent git
  commits for the public channel), then `{"type":"msg","message":Message}`
  per new message. `Message = {id, user, color, text, kind, time}`, `kind`
  one of `"user"`, `"system"`, `"git"`.
- **Client → server**: `{"type":"msg","text":"..."}`. Text is
  control-character-sanitized server-side before it's ever stored or
  broadcast, so a message can never inject extra lines into `.wede/chat.md`.
- Identity (`username`) comes from the session; `color` is client-chosen and
  purely cosmetic (default `#888888`).
- Read limit 4 KiB per frame; ping every 30 s.

### CRDT document sync — `GET /api/workspaces/{id}/doc/{room...}`

- `{room...}` decodes (base64url, no padding) to the file's
  workspace-relative path.
- The actual message framing on this socket is the standard **Yjs
  sync + awareness wire protocol**, implemented by the third-party
  `github.com/reearth/ygo/provider/websocket` package — wede does not define
  custom framing here, so this document won't restate that library's wire
  format. What wede *does* own is persistence: a new document is seeded from
  the file's on-disk content on first open (`LoadDoc`), and edits are
  debounced 600 ms then written back to disk as the document's fully
  materialized text via an atomic temp-file-and-rename (`StoreUpdate` →
  `DiskPersistence`).
- Editor+ only — this socket drives writes to workspace files.
