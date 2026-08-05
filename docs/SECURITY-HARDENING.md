# Hardening a wede deployment

This is the checklist to work through before you let anyone other than
yourself, on a machine other than your laptop, reach a wede instance.
Everything here is also mentioned somewhere in [README.md](../README.md),
[GETTING-STARTED.md](GETTING-STARTED.md), [CONFIGURATION.md](CONFIGURATION.md),
[PUBLIC-ACCESS.md](PUBLIC-ACCESS.md), or [ARCHITECTURE.md](ARCHITECTURE.md) —
this page exists so you don't have to assemble it yourself from five places.

## Threat model — read this part

wede's security model is deliberately simple, and simple has sharp edges.
Know them before you deploy.

- **There are no user accounts.** Authentication is one owner password
  (`wede.config.json`) plus revocable share tokens minted by the owner
  (`POST /api/auth/tokens`). There is no IdP, no per-user login, no SSO. Every
  "editor" or "viewer" is just a bearer of a token the owner handed out —
  wede has no idea who they actually are beyond a self-chosen display name.

- **An editor share link is an unsandboxed login shell on your machine.**
  This is the single most important fact on this page. Connecting to the
  terminal WebSocket (`GET /api/workspaces/{id}/terminal`) runs
  `exec.Command(shell, "-l")` — the OS user's real login shell, `-l` flag and
  all — as the OS user that started the wede process
  (`backend/internal/terminal/terminal.go`, `getOrCreateSession`). The child
  inherits the full process environment (`cmd.Env = append(os.Environ(), ...)`)
  and the shell's working directory is set to the workspace root only as a
  starting point (`cmd.Dir = dir`) — nothing stops the session from `cd`-ing
  anywhere, reading `~/.ssh`, running `sudo`, or doing anything else that OS
  user can do. `roles.go` confirms editor and owner are the only roles that
  can reach this route (`RequireEditor` gates it; `CanMutate()` returns true
  for `RoleOwner` and `RoleEditor` only). The LSP and DAP WebSockets are the
  same story: connecting spawns a real process (`gopls`, `rust-analyzer`,
  `dlv dap`, ...) as a child of wede with full filesystem and network access,
  and are also `RequireEditor`-gated for exactly this reason.

  **Treat every editor share link like an SSH private key to your account.**
  Mint one per person, not one shared link for a team. Never post one in a
  group chat, ticket, or anywhere it could be forwarded.

- **A viewer share link is read-only — except chat.** `roles.go` defines
  `RoleViewer` as unable to call `CanMutate()`; every mutating, terminal, LSP,
  DAP, and CRDT-doc route in `main.go` is wrapped in `RequireEditor`, so a
  viewer gets a `403` on all of them — no shell, no file writes, no git
  mutations. The one deliberate exception is the per-workspace chat socket
  (`GET /api/workspaces/{id}/chat`): it is mounted with no `RequireEditor`
  wrapper because chat is designed as a public channel any authenticated role
  can use, so a viewer **can** post messages, which are appended to the
  committed `.wede/chat.md` (public channel) after control-character
  sanitization. A viewer cannot write any other file.

- **wede's own listener is plain HTTP.** `backend/cmd/wede/main.go` calls
  `http.ListenAndServe(addr, securityHeaders(cfg, mux))` — there is no TLS
  code path in the binary at all. Encryption in transit is entirely your
  responsibility: either a reverse proxy terminating TLS in front of wede
  (see [PUBLIC-ACCESS.md](PUBLIC-ACCESS.md) option (a)), or the built-in
  tunnel's `wss://` connection to your own relay (option (c)). If you expose
  wede over plain `http://` on anything other than loopback or a network you
  fully trust, the owner password, session tokens, and every keystroke of
  every terminal session cross the wire in the clear.

With that in mind, the checklist:

## Hardening checklist

### 1. Use a strong owner password, and change the shipped placeholder

`wede.config.example.json` ships with `"password": "CHANGE_ME_BEFORE_USE"`.
`config.Load()` (`backend/internal/config/config.go`) refuses to start with an
empty password, but it does **not** refuse the example placeholder — if you
copy the example file into place without editing it, wede will run with a
password anyone can find in the public repo. Set a long, random password.

Login is also brute-force-limited: `auth.Handler` allows 3 wrong attempts
(`maxAttempts: 3`) before locking out further login attempts with `403
{"error":"locked"}`, and the lockout state is persisted to
`~/.wede/lockout.json` so restarting wede does not reset the counter. The only
way to unlock is to delete that file (the error message tells you this
directly) or wait for the owner to do so — there's no time-based auto-unlock.

### 2. Keep the default loopback bind unless you have a reason not to

`config.Load()` defaults `Host` to `127.0.0.1`. Setting `"host": "0.0.0.0"`
(or any non-loopback address) in `wede.config.json` makes wede reachable from
other machines directly, with no proxy in between — fine on a trusted LAN,
risky on anything internet-facing. If you need public reachability, prefer a
reverse proxy or the tunnel (both keep wede itself on loopback) over binding
wide open — see [PUBLIC-ACCESS.md](PUBLIC-ACCESS.md).

### 3. Confine `workspace_root`

`WorkspaceRoot` (config key `workspace_root`, env override
`WEDE_WORKSPACE_ROOT`) is the base directory `POST /api/folder/open` and
`POST /api/workspaces` are allowed to open, enforced by
`folder.ValidateRoot()`. It defaults to `$HOME` and always rejects the
allowed-base itself, the filesystem root, and any dotfile path component
(`.ssh`, `.gnupg`, `.config`, ...) even inside the allowed base. If your
deployment only ever needs one or two project trees, point `workspace_root`
at a narrower directory than `$HOME` so an editor session can't adopt an
unrelated part of your filesystem as a workspace — though given point 1 of
the threat model, an editor already has shell access to everything the OS
user can reach regardless of this setting. This restricts what the *file/git
API* can touch, not what a terminal session can touch.

### 4. Run wede as a dedicated, unprivileged OS user

Because an editor share link is equivalent to a shell as the process owner
(see the threat model above), the single biggest lever you have is *who that
OS user is*. Don't run wede as your own primary user, and never as root.
Create a dedicated account with only the access wede's workspaces actually
need, and run the binary under it (a systemd user unit or an unprivileged
system service both work). This turns "an editor link is a shell as you"
into "an editor link is a shell as a low-privilege account that can only see
the projects you gave it."

### 5. Put TLS in front of it

wede has no TLS of its own (see the threat model above). Use a reverse proxy
(Caddy or nginx, both documented with working configs) or the built-in
tunnel, which dials out over `wss://` to your own relay. Full walkthroughs
for both, plus the tradeoffs against a generic outbound tunnel (Cloudflare
Tunnel / ngrok / frp / Tailscale Funnel), are in
[PUBLIC-ACCESS.md](PUBLIC-ACCESS.md). If your proxy forwards WebSocket
upgrades incorrectly, the terminal/LSP/DAP/collab/chat features will silently
fail to connect — the sample configs in PUBLIC-ACCESS.md include the
`Upgrade`/`Connection` headers this requires.

### 6. Only set `frame_ancestors` for an origin you actually control

`frame_ancestors` (empty by default) controls whether wede can be embedded in
an `<iframe>`. `securityHeaders()` in `main.go` emits, by default,
`X-Frame-Options: DENY` and `Content-Security-Policy: frame-ancestors 'self'`
— all cross-origin framing is blocked. Setting `frame_ancestors` to a
space-separated origin list (e.g. `"https://vulos.org"`) switches to emitting
only `Content-Security-Policy: frame-ancestors <your list>` (the
`X-Frame-Options` header is dropped so the CSP directive takes sole effect),
and that same allowlist is threaded into the terminal/LSP/DAP/collab/chat
WebSocket origin checks (`checkOrigin()` in each package). Only list an
origin you control and trust to embed wede — anything in that list can frame
your IDE and, if it can also get a browser to carry a valid session, ride the
clickjacking surface framing exists to prevent.

### 7. Practice share-token hygiene

Share tokens are minted by the owner via `POST /api/auth/tokens` (owner-only,
`{"role":"viewer"|"editor","username":"...","ttlHours":0}`) and are the only
way anyone besides the owner gets in. The token model
(`backend/internal/auth/tokens.go`):

- 32 random bytes (`crypto/rand`), hex-encoded — effectively unguessable.
- Only the SHA-256 hash is persisted to `~/.wede/tokens.json`; the raw token
  is returned exactly once, in the mint response.
- Redemption (`RedeemToken`) compares hashes with `subtle.ConstantTimeCompare`
  — no timing side channel.
- `ttlHours: 0` (the default if omitted) means the token **never expires**.
  Set an explicit TTL for anything you're not prepared to track forever.
- Revocation is immediate: `DELETE /api/auth/tokens/{id}` deletes the token
  from the map and disk; once gone it can never be redeemed again (though
  sessions already redeemed from it keep working until their own 24 h TTL
  expires — revoking the token stops *new* redemptions, not existing
  sessions).
- The public redemption endpoint (`POST /api/auth/redeem`) is rate-limited to
  10 attempts per client IP per minute (`redeemMaxPerWindow` /
  `redeemWindow`), so a brute-force guess against a live token is slow but
  not impossible against a weak deployment — the token's own 256 bits of
  entropy is what actually protects you.

Given point 2 of the threat model, mint **editor** tokens as rarely as
possible and only for people you'd hand a shell account to; default to
**viewer** whenever read access is all that's needed.

### 8. Understand session handling

Sessions (from either password login or token redemption) last 24 hours of
idle time (`auth.SessionTTL`), are stored keyed by their SHA-256 hash in
`~/.wede/sessions.json` (mode `0600`), and are pruned lazily on load and on
each validity check. `DELETE /api/auth/logout` revokes the caller's session
server-side immediately (not just client-side cookie clearing). There is no
"log out all sessions" — revoking a share token only stops future
redemptions of that token, and an owner who suspects their own password is
compromised should change the password (which does not itself invalidate
existing owner sessions, since sessions aren't tied to the password after
issuance — deleting `~/.wede/sessions.json` while wede is stopped is the
blunt-force way to kill every active session at once).

### 9. Only trust workspaces you've audited

A cloned repo can ship `.wede/tasks.json`, `.wede/formatters.json`, and
`.wede/debug.json` — and each of those causes wede to **run a host command**
(a task, a formatter piped through stdin/stdout, a debug adapter binary).
`backend/internal/trust/trust.go` gates all three: project-level config from
a workspace root is only honored once the owner has explicitly called
`POST /api/workspaces/{id}/trust` for that root (persisted to
`~/.wede/trusted.json`); until then, only the owner's own global
`~/.wede/{tasks,formatters,debug}.json` apply. This exists specifically so
that an editor-role collaborator can't get the owner to execute arbitrary
commands by committing a malicious `.wede/tasks.json` into a shared repo.
**Never trust a workspace whose `.wede/` config you haven't read** — trusting
it is explicitly telling wede "run whatever commands this repo's committed
config says to run."

### 10. Know what the API-client SSRF guard does and doesn't cover

The built-in HTTP API client (`backend/internal/apiclient/apiclient.go`) lets
an editor send arbitrary HTTP requests server-side (no browser CORS limits).
By default, `safeDialContext` resolves the target hostname and rejects the
request if the resolved IP is loopback, private (RFC1918/ULA), link-local
(including the `169.254.169.254` cloud-metadata address), or unspecified —
checked **after** DNS resolution, which defeats DNS-rebinding attacks that a
hostname-based check would miss. Setting
`WEDE_APICLIENT_ALLOW_PRIVATE=1` (or `true`/`yes`/`on`) disables this guard
entirely for the whole process. Only set it if you specifically need the API
client to reach your own loopback/LAN services (e.g. a local dev server) —
doing so also means an editor-role collaborator can use the API client to
probe or pivot into loopback and private-network services from the wede
host, including any cloud metadata endpoint if you're on a cloud VM.

### 11. Understand what the tunnel does and doesn't expose

The owner-only public tunnel (`GET/PUT/POST /api/tunnel/...`, all
`RequireOwner`) dials a single outbound `wss://` connection to a relay server
*you* run, authenticates with a bearer token, and proxies inbound requests to
wede's own loopback address — `tunnel.New()` in `main.go` pins the local
target to `"127.0.0.1:" + cfg.Port` regardless of `cfg.Host`, so the tunnel
can never be pointed at an arbitrary host even if wede itself is bound wider.
The tunnel config, including the relay bearer token, is persisted to
`~/.wede/tunnel.json` (mode `0600`); `Manager.Snapshot()` redacts the token
before it's ever returned to the owner UI, so it never round-trips back to
the browser. Anyone who reads `~/.wede/tunnel.json` off disk gets a live
credential to your relay — protect that file's permissions as carefully as
the config's password. See [PUBLIC-ACCESS.md](PUBLIC-ACCESS.md) for what the
relay operator can and cannot see of your traffic (it depends on which relay
you run — this is your own infrastructure, not a Vulos-hosted service).

### 12. Remember your backups contain secrets

`~/.wede/sessions.json`, `~/.wede/tokens.json`, `~/.wede/tunnel.json`, and
`~/.wede/lockout.json` all contain sensitive state (hashed credentials, a
live relay bearer token, lockout counters). Per-workspace `.wede/private/`
holds the private chat channel, which wede deliberately keeps out of git via
an auto-written `.gitignore` entry — a filesystem-level backup will still
pick it up. If your backup strategy copies `~/.wede/` or workspace `.wede/`
directories somewhere less trusted than the wede host itself (an
unencrypted cloud backup, a shared NAS volume, etc.), you've extended the
trust boundary to that location too.

### 13. Keep the binary current, and know where to report a problem

wede is a single static binary — updating means replacing it and restarting.
Watch the [releases page](https://github.com/vul-os/wede/releases). To report
a vulnerability, do **not** open a public issue
— use [GitHub private vulnerability reporting](https://github.com/vul-os/wede/security/advisories/new)
or email `vulosorg@gmail.com` with `[wede security]` in the subject, per
[SECURITY.md](../SECURITY.md).

## What wede does NOT protect against

Be honest with yourself and anyone you grant access about these gaps —
they're inherent to wede's single-owner, no-accounts design, not bugs:

- **No per-file or per-directory permissions.** Roles are workspace-wide:
  owner, editor, or viewer for the whole session, not "editor on this folder,
  viewer on that one."
- **No sandboxing of a collaborator's shell, LSP, or DAP session.** As
  covered above, an editor gets the real OS user's shell and real host
  processes — no container, no chroot, no seccomp profile around any of it.
- **No audit log of who ran what.** wede does not record terminal input,
  executed commands, or which token performed a given file/git mutation
  beyond what's visible live in presence/chat. If you need "who did this,"
  you won't find it in wede.
- **No multi-factor authentication.** Login is a single password (or a
  bearer share token) — there is no second factor of any kind.
- **No rate limiting on password login beyond the 3-attempt lockout.** There
  is no per-IP throttling on `POST /api/auth/login` distinct from the global
  lockout counter — three wrong passwords lock out login for everyone until
  the owner deletes `~/.wede/lockout.json`, which is itself a workable
  denial-of-service against your own login if wede is reachable by anyone
  who wants to lock you out.
