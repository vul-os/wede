<!-- Generated from docs/TROUBLESHOOTING.md by scripts/sync-docs.mjs — edit the source, not this file. -->

# Troubleshooting & FAQ

Symptom → cause → fix, grounded in what the code actually does (file paths
point at `backend/internal/...` in the wede repo). If something here doesn't
match what you're seeing, check the [CHANGELOG](#changelog) for your
version before assuming a bug.

---

## Locked out after failed logins

**Symptom:** Login returns "Too many failed attempts" (HTTP 403, `{"error":
"locked", ...}`) even with the correct password.

**Cause:** wede locks the login endpoint after **3** failed password attempts
(`maxAttempts` in `backend/internal/auth/auth.go`) and persists that state to
`~/.wede/lockout.json` so a server restart alone does **not** clear it.

**Fix:**

1. Stop wede.
2. Delete (or reset) the lockout file:
   ```bash
   rm ~/.wede/lockout.json
   ```
   (`~` here is the home directory of the OS user running wede — see
   [DEPLOYMENT.md § State & backup](#deployment:5-state--backup) if you're
   running it as a dedicated service user, not your own login user.)
3. **Restart wede.** This step is not optional: `lockout.json` is only ever
   *read* once, at `Handler` construction (`loadLockout`, called from
   `auth.NewWithDataDir`). Deleting the file while wede keeps running has no
   effect — the in-memory `locked` flag stays `true` until the process
   restarts and re-reads (or fails to find) the file.

There is no API endpoint to reset the lockout remotely — it's a deliberate
owner-only, filesystem-level control.

---

## Forgot / need to change the owner password

There's only one password — the one in `wede.config.json` (`password` key,
required, no default). To change it:

1. Find which config file is actually in use — wede logs it at startup:
   `loaded config from <path>` (`backend/internal/config/config.go`). It
   searches, in order: the working directory and its parents, then
   `~/.config/wede/wede.config.json`, then next to the binary — the first one
   found wins, so if you have more than one lying around, the log line tells
   you which one is live.
2. Edit the `password` field and restart wede.

**Strict parsing means typos break startup, not silently ignore your
change.** The config decoder uses `json.Decoder.DisallowUnknownFields()`, so
an unrelated typo elsewhere in the file (e.g. `"frame_ancestor"` instead of
`"frame_ancestors"`) makes wede refuse to start at all with `invalid
wede.config.json: json: unknown field "..."` — it will not start up with the
old password as a fallback. Check `journalctl -u wede` (or wherever your
service manager sends stderr) for that exact message if wede won't come back
up after an edit.

Changing the password does **not** invalidate already-logged-in sessions or
outstanding share links — both are checked independently of the owner
password after they're issued (see `backend/internal/auth/tokens.go`). If you
suspect a session or share link is compromised, revoke it explicitly
(**Settings → Share links**, or log out) rather than relying on a password
change to do it for you.

---

## Port already in use / can't reach it from another machine

**Port in use:** something else already has the port (default `9090`).
Either stop that process or start wede on a different one:

```bash
wede --port 8080 /path/to/project
```

(`--port`/`-p` overrides the config file's `port` key — see
[CONFIGURATION.md § CLI flags](#configuration:cli-flags).)

**Can't reach it from another machine (but it works on `localhost`):** this
is expected, not a bug. wede binds to **`127.0.0.1` by default** — loopback
only, not reachable from anywhere else on the network, even by design
(`config.parse` in `backend/internal/config/config.go` sets `Host:
"127.0.0.1"` as the default). To reach it from another machine you must
explicitly set:

```json
{ "host": "0.0.0.0" }
```

Before you do that on anything but a fully trusted LAN, read
[PUBLIC-ACCESS.md](#public-access) — binding to `0.0.0.0` puts a
plaintext-HTTP, single-password login page on every interface the machine
has, and the doc walks through the safer options (reverse proxy with TLS,
an outbound tunnel, or wede's own Ephor relay) instead of exposing the raw
port directly.

---

## `/site/` or the docs return 404 on my own instance

**This is expected.** `/site/*` (the marketing landing page + embedded docs
viewer) is gated by the `serve_landing` config key, which **defaults to
`false`** (`backend/cmd/wede/main.go`, `backend/internal/config/config.go`).
It's built for the public cloud deployment (`wede.vulos.org`); a normal
self-hosted instance has no use for it and gets a plain 404 at `/site/` —
that's not a missing-file bug, it's the route simply not being mounted.

Unauthenticated visits to `/` still work correctly either way — you get the
IDE's own login form (the SPA), not a broken page. If you *want* the landing
page and docs viewer (e.g. you're running a public-facing instance and like
that UI), set `"serve_landing": true` in `wede.config.json`.

One route is **not** gated by `serve_landing` and is always available:
`GET /licenses.txt` (third-party notices), mounted unconditionally whenever
the binary was built with notices embedded.

---

## Autocomplete/hover doesn't work for language X

**Cause:** wede does not bundle any language servers. LSP support is a proxy
(`backend/internal/lsp/lsp.go`) that locates binaries via `exec.LookPath` —
if the binary isn't installed and on the wede process's `PATH`, there's
nothing to proxy to.

**Built-in (auto-detected if installed):**

| Language | Binary |
|---|---|
| Go | `gopls` |
| JavaScript/TypeScript | `typescript-language-server` |
| Python | `pylsp` |
| Rust | `rust-analyzer` |

Check **Settings → Language server** (or `GET
/api/workspaces/{id}/lsp/available`) to see what wede actually found on
`PATH`. If a server isn't installed, wede degrades gracefully — no crash, no
HTTP 500 — it opens the WebSocket, sends one informational
`window/showMessage` notification, and closes.

**Add any other LSP server** without recompiling via `~/.wede/lsp.json`:

```json
{
  "servers": {
    "c": { "command": "clangd", "extensions": ["c", "h"] }
  }
}
```

`LoadConfig` merges this into the built-in registry at startup
(`backend/cmd/wede/main.go`), overriding a built-in of the same name. **This
registry is global-only today** — `~/.wede/lsp.json`, not a per-project one.
`trust/trust.go` lists `lsp.json` among the files that make a workspace
"worth prompting to trust," but no code path currently reads a project-local
`.wede/lsp.json`; [GETTING-STARTED.md](#getting-started:workspace-trust)
says outright that "project `.wede/lsp.json` support lands with the same
trust gate" — i.e. it's planned, not shipped. Restart wede after editing
`~/.wede/lsp.json` for changes to take effect.

**Debugging (DAP)** follows the same shape, and — unlike LSP — *is* wired up
per-project: built-ins are `dlv` (Go, via `dlv dap`) and `debugpy-adapter`
(Python); add more via the global `~/.wede/debug.json` **or** a trusted
project's `.wede/debug.json` (`backend/internal/dap/dap.go`).

**Formatters** also follow this shape and are also project-aware: built-ins
are `gofmt` (Go), `prettier` (JS/TS/CSS/JSON/HTML/Markdown), and `black`
(Python); add more via the global `~/.wede/formatters.json` **or** a trusted
project's `.wede/formatters.json` (`backend/internal/files/format_config.go`).

---

## Format-on-save / tasks don't run

**Cause:** this is almost always the **workspace trust gate**
(`backend/internal/trust/trust.go`), not a broken formatter/task. Tasks
(`~/.wede/tasks.json`) and formatters (`~/.wede/formatters.json`) that live
in your **global** `~/.wede/` always apply. But a *project-committed*
`<workspace>/.wede/tasks.json` or `.wede/formatters.json` runs a host
command that ships inside a repo — potentially one written by a
collaborator, not you — so it is **ignored until the owner explicitly
trusts that workspace**.

**Fix:** as the owner, go to **Settings → Workspace trust** and trust the
workspace (or `POST /api/workspaces/{id}/trust`). `GET
/api/workspaces/{id}/trust` tells you both the current trust state and
whether the workspace even ships trust-gated config
(`hasProjectConfig`), so you can tell "nothing to trust" apart from
"not yet trusted." Untrusting (`DELETE`) revokes it immediately. Only trust
workspaces whose collaborators — and their commit history — you actually
trust; this gate exists specifically to stop an editor-role collaborator from
running arbitrary host commands as the owner via a committed config file.

---

## Git features missing or erroring

wede has no embedded git implementation — every git operation shells out to
the system `git` binary (`exec.Command("git", ...)` in
`backend/internal/git/git.go`). If `git` isn't installed or isn't on the
`PATH` wede's process sees, git operations fail.

**A specific gotcha:** the status/branch handlers treat *any* failure of `git
status --porcelain` — including "git binary not found" — identically to "this
directory isn't a git repository" and quietly return `{"isRepo": false,
"branch": "", "files": []}` (`git.go`, `Status`). So if the Git panel insists
a real git repository "isn't a repo," check that `git` is actually installed
and on `PATH` for the wede process (not just your interactive shell's PATH —
double-check what a systemd/launchd-launched process sees) before assuming
your repo is somehow broken.

---

## Search is slow / results differ from what you expect

wede uses **ripgrep (`rg`)** when it's found on `PATH`, and falls back to a
pure-Go directory walk when it isn't (`backend/internal/search/search.go`,
`exec.LookPath("rg")`). Installing `rg` is the fix for both slowness and
coverage gaps:

- **The Go fallback only searches files whose extension is on a fixed
  allowlist** (`textExtensions` in `search.go` — common languages, configs,
  markdown, etc.) and skips a fixed set of directories (`.git`,
  `node_modules`, `.cache`, `vendor`, `dist`, `.next`, `build`). It does
  **not** read your project's `.gitignore` at all. `rg`, by contrast,
  respects `.gitignore` by default and additionally hard-excludes
  `.git/`/`node_modules/` — so a file gitignored for a reason other than
  those two directories may show up in fallback results but not in `rg`
  results, and a file with an extension not in `textExtensions` (e.g. an
  uncommon language) is silently skipped by the fallback but would be
  searched by `rg`.
- **Both backends cap results:** 500 matches for text search, and
  replace-across-files is capped at 200 files / 10,000 total replacements —
  past that you get `truncated: true` or an explicit "too many files" /
  "too many replacements" error rather than an unbounded operation. This is
  a deliberate ceiling, not a bug, on very large repos or very broad queries.
- Files over 2 MiB are skipped entirely by the Go fallback.

---

## WebSockets fail behind a reverse proxy

wede's terminal, LSP, DAP, collab-presence, and CRDT-doc features are all
WebSocket-based. A reverse proxy that doesn't forward the WebSocket upgrade
handshake will make all of them silently fail while the rest of the app
(file browsing, git, static assets) works fine — that split is the tell.

At minimum your proxy needs to forward the `Upgrade` and `Connection`
headers and not buffer the connection. See
[PUBLIC-ACCESS.md § Direct bind + reverse proxy](#public-access:a-direct-bind--reverse-proxy)
for working Caddy and nginx configs — Caddy's `reverse_proxy` handles
upgrades automatically; nginx needs the explicit
`proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection
"upgrade";` lines shown there. Also check that any proxy-level idle timeout
is generous — wede's terminal keeps a WebSocket open indefinitely with a
25-second ping/60-second read-deadline keepalive
(`backend/internal/terminal/terminal.go`), so an aggressive proxy timeout
shorter than that will drop idle terminals.

---

## Embedding in an iframe shows a blank page / is blocked

**Cause:** by default wede sends `X-Frame-Options: DENY` and
`Content-Security-Policy: frame-ancestors 'self'`
(`securityHeaders()` in `backend/cmd/wede/main.go`) — this **blocks all
cross-origin iframe embedding** on purpose, so a standalone wede isn't
embeddable by some third-party page without your say-so.

**Fix:** set `frame_ancestors` in `wede.config.json` to the origin(s) that
should be allowed to embed it:

```json
{ "frame_ancestors": "https://vulos.org" }
```

When `frame_ancestors` is non-empty, wede emits `Content-Security-Policy:
frame-ancestors <value>` instead and **omits** `X-Frame-Options` (it can't
express multiple origins), and the WebSocket origin checks for
terminal/LSP/collab widen to accept the same listed origins. The value is
space-separated for multiple origins. See
[CONFIGURATION.md § frame_ancestors](#configuration:frame_ancestors).
Direct (non-embedded) browser access is unaffected either way.

---

## API client can't reach my local service

**Cause:** the built-in API client's server-side send proxy has an SSRF
guard on by default (`backend/internal/apiclient/apiclient.go`). Requests are
DNS-resolved and the **resolved IP** — not the hostname — is checked against
loopback, RFC1918/ULA private ranges, link-local (including the
`169.254.169.254` cloud-metadata address), and unspecified addresses;
blocked ranges are rejected before the connection is made. Checking
post-resolution (not the hostname string) is deliberate — it closes the
DNS-rebinding hole where a hostname that resolves to a public IP at request-build
time is repointed at a private IP by the time the request actually fires.

This means calling `http://localhost:3000` or `http://192.168.1.50:8080` from
the API client fails by default — including your own dev server on the same
box.

**Fix, with the risk stated plainly:** set
`WEDE_APICLIENT_ALLOW_PRIVATE=1` (or `true`/`yes`/`on`) in wede's process
environment and restart:

```bash
WEDE_APICLIENT_ALLOW_PRIVATE=1 ./wede /path/to/project
```

or in the systemd unit: `Environment=WEDE_APICLIENT_ALLOW_PRIVATE=1`. This
re-opens the API client to loopback/private/metadata targets — appropriate
when you're intentionally testing against your own local service, but it
also means anyone with editor access can use the API client to probe
whatever else is reachable on your private network or cloud metadata
endpoint from the wede host. Only set it if you understand and accept that.

---

## Collaborators can't join / share link rejected

A few distinct failure modes, all in `backend/internal/auth/`:

- **`"invalid or expired token"` (401) on redeem** — the token was revoked
  (`DELETE /api/auth/tokens/{id}`, owner-only), or it was minted with a
  `ttlHours` and that time has passed. Tokens minted with `ttlHours: 0` (or
  omitted) never expire; check **Settings → Share links** for the token's
  actual expiry, or re-mint a new one.
- **`"too many attempts; try again later"` (429) on redeem** — the public
  `/api/auth/redeem` endpoint rate-limits to **10 attempts per client IP per
  minute** (`redeemMaxPerWindow`/`redeemWindow` in
  `backend/internal/auth/handlers.go`). This throttles brute-forcing of
  invite tokens; it also means several people behind the same NAT'd/office IP
  redeeming links in quick succession can trip it. Wait a minute and retry.
- **Editor link "works" but the person doesn't get a shell/writes** — check
  the role the token was minted with (viewer vs. editor); viewers are
  correctly rejected with 403 from terminal, LSP, DAP, file-write, and git-
  mutation routes by design (`RequireEditor` in `backend/internal/auth/auth.go`).
  This is not a bug — see
  [ARCHITECTURE.md § Role model](#architecture:role-model).

---

## "Can I open a folder outside my home directory?"

Yes, via the `workspace_root` config key (or `WEDE_WORKSPACE_ROOT` env var),
which sets the base directory that runtime folder/workspace-open requests
are confined to (default: `$HOME`). See
[CONFIGURATION.md § workspace_root](#configuration:workspace_root) and
[DEPLOYMENT.md § workspace_root](#deployment:workspace_root-what-it-confines-and-what-it-doesnt)
for the exact rules (no filesystem root, no dotfile path segments, no
traversal) and — importantly — what this setting does **not** do: it
restricts which directories the workspace UI/API will open, it does not
sandbox the shared terminal's shell, which can `cd` anywhere the OS user
running wede can regardless of `workspace_root`.

One exception either way: the directory you pass on the command line at
launch (`wede /path/to/project`) is the owner's explicit boot choice and
is exempt from the `workspace_root` check entirely.

---

## Other things worth knowing

- **`~/.wede/lsp.json` needs a wede restart to take effect; `formatters.json`,
  `tasks.json`, and `debug.json` do not.** `LoadConfig` for LSP servers runs
  once at startup and mutates a package-level registry
  (`backend/cmd/wede/main.go`); formatters, tasks, and debug adapters are all
  re-read from disk on each relevant request (`files/format_config.go`,
  `tasks/tasks.go`, `dap/dap.go`) — edit them and the next save/task-list/
  debug-start picks the change up immediately, no restart needed.
- **Deleting `~/.wede/trusted.json` un-trusts every workspace at once** —
  harmless, but you'll need to re-trust each one before its committed
  tooling config runs again.
- **The public tunnel only ever proxies to wede's own loopback address**,
  regardless of what `host` is set to in your config
  (`tunnelMgr := tunnel.New("127.0.0.1:" + cfg.Port)` in `main.go`) — if the
  tunnel won't start, check the relay `serverUrl`/token/name in **Settings →
  Public access**, not your `host` config.
- **Response bodies from the API client's send proxy are capped at 10 MiB**
  (`maxResponseBytes` in `apiclient.go`); a larger response is truncated, not
  rejected outright.

---

## See also

- [DEPLOYMENT.md](#deployment) — running wede as a long-lived service,
  what state it keeps, and how to back it up.
- [PUBLIC-ACCESS.md](#public-access) — exposing wede beyond `localhost`
  safely.
- [CONFIGURATION.md](#configuration) — every config key, defaults, and
  precedence rules.
- [ARCHITECTURE.md](#architecture) — the security/role model in full.
