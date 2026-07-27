# Running wede as a service

This guide is for anyone putting wede on a server, a NAS, or a Raspberry Pi and
wanting it to survive reboots — as opposed to running `./wede /path/to/project`
in a terminal you keep open. It covers where to put the binary, which OS user
should run it, a systemd unit, a launchd equivalent for macOS, notes on
Windows, what state wede keeps on disk and how to back it up, and how to
upgrade in place.

It assumes you've already read [GETTING-STARTED.md](GETTING-STARTED.md) and
have a working `wede.config.json`. For TLS / reverse proxies / tunnels, see
[PUBLIC-ACCESS.md](PUBLIC-ACCESS.md); for the full config key reference, see
[CONFIGURATION.md](CONFIGURATION.md).

> wede's [README](../README.md) currently carries a **"deprioritized" /
> community-maintained** status note. None of that changes what's below — the
> binary, its config, and its on-disk state are what they are — but if you're
> deciding whether to depend on it long-term, read that note first.

---

## 1. Choosing where it runs

### Read the security model first

**An editor share link is not a scoped IDE permission — it's a login shell.**
wede's own docs are blunt about this, and this guide won't soften it:

> When you share an **editor** link, the recipient gains access to a full
> login shell (`exec.Command(shell, "-l")`) running as the OS user that
> started wede, with the complete process environment inherited
> (`os.Environ()`). The working directory is set to the workspace root as a
> UX convenience only — it does **not** confine the shell in any way. The
> editor can `cd /`, read `~/.ssh`, run `sudo`, install packages, or do
> anything the wede process owner can do.

— [ARCHITECTURE.md § No-sandbox warning](ARCHITECTURE.md#no-sandbox-warning-for-shared-deployments),
also called out in the [README](../README.md#collaboration).

The same is true of the LSP and DAP WebSockets: connecting to either spawns a
language-server or debug-adapter binary as a child of the wede process, with
the same filesystem and network access as the wede user. This is by design —
wede is a developer tool, not a sandboxed IDE-as-a-service — but it means
**the OS user you run wede as is the actual blast radius of every editor
share link you hand out.**

### Pick a dedicated, unprivileged user

Don't run wede as your own login user or as `root`. Create a user that owns
nothing you'd mind an editor-role collaborator touching:

```bash
sudo useradd --system --create-home --home-dir /srv/wede --shell /usr/sbin/nologin wede
```

- `--create-home` gives it its own `~/.wede/` for sessions/tokens/lockout
  state and its own `$HOME` for the workspace-root default (see below).
- `--shell /usr/sbin/nologin` stops *you* from `su -`-ing into it casually, but
  has **no effect** on wede's own terminal feature — that spawns `$SHELL -l`
  from the process's own `$SHELL` environment variable (or `/bin/bash` if
  unset — see [terminal.go](../backend/internal/terminal/terminal.go)), not
  via the user's login shell field in `/etc/passwd`. Set `Environment=SHELL=
  /bin/bash` (or whatever you want editors to get) in the systemd unit below
  if you want to control this explicitly.
- Never run wede as a user that has passwordless `sudo`, is in the `docker`
  group, or has SSH keys to other machines — all of those are directly
  reachable from any editor's shared terminal.

If you're hosting several unrelated people's projects on one box, give each
its own OS user and its own wede instance/port. One wede process = one shared
security boundary = the OS user it runs as.

### `workspace_root`: what it confines and what it doesn't

`workspace_root` (config key, `backend/internal/config/config.go`; also
settable via the `WEDE_WORKSPACE_ROOT` env var) is the directory tree that
runtime folder/workspace operations are confined to:

```json
{
  "password": "…",
  "workspace_root": "/srv/projects"
}
```

Any path passed to `POST /api/folder/open` or `POST /api/workspaces` must
resolve *inside* `workspace_root` and must not be the filesystem root, the
user's home directory itself, or contain a dotfile path segment (`.ssh`,
`.config`, …). It defaults to `$HOME` if unset. See
[CONFIGURATION.md § workspace_root](CONFIGURATION.md#workspace_root) for the
full rule set, including the one exception: the path you pass on the command
line at launch (`wede /path/to/project`) is the owner's explicit boot choice
and is exempt from this check.

**What it is:** a guard against an authenticated editor pointing the file
browser/workspace UI at a sensitive directory (`$HOME`, `/`, `~/.ssh`) and
reading it through wede's file/git/search APIs.

**What it is not:** a sandbox. It restricts which directories the *workspace
UI* will open — it does nothing to the shared terminal's shell, which can
`cd` anywhere the OS user can, regardless of `workspace_root`. Treat
`workspace_root` as tidiness/defense-in-depth for the file-browsing surface,
not as the thing standing between an editor link and the rest of the disk.
The dedicated-user boundary above is what actually matters.

---

## 2. systemd unit (Linux)

Build the binary first (`npm run build:all`, see the main
[README](../README.md#quick-start)) and place it somewhere stable, e.g.
`/usr/local/bin/wede`. Put the config where the dedicated user's config
search will find it — `~/.config/wede/wede.config.json` under the service
user's home is the cleanest choice since it needs no `WorkingDirectory`
coupling (see [CONFIGURATION.md § Config file location](CONFIGURATION.md#config-file-location)
for the full search order: cwd and parents, then `~/.config/wede/`, then next
to the binary).

```bash
sudo mkdir -p /srv/wede/.config/wede
sudo tee /srv/wede/.config/wede/wede.config.json >/dev/null <<'EOF'
{
  "password": "change-me-to-something-strong",
  "port": "9090",
  "host": "127.0.0.1"
}
EOF
sudo chown -R wede:wede /srv/wede
sudo chmod 600 /srv/wede/.config/wede/wede.config.json
```

`/etc/systemd/system/wede.service`:

```ini
[Unit]
Description=wede — self-hosted collaborative web IDE
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=wede
Group=wede
WorkingDirectory=/srv/wede
Environment=SHELL=/bin/bash
Environment=WEDE_WORKSPACE_ROOT=/srv/projects
ExecStart=/usr/local/bin/wede /srv/projects
Restart=on-failure
RestartSec=5

# ── Hardening that is SAFE here ──────────────────────────────────────────
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/srv/wede /srv/projects
ProtectHome=read-only
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true

[Install]
WantedBy=multi-user.target
```

Notes on the unit:

- `Type=simple` is correct — `main.go` calls `http.ListenAndServe` directly
  and never forks or double-forks.
- `ExecStart` passes the boot workspace as a positional argument (`wede
  [path]`, `backend/cmd/wede/main.go`); omit it if you'd rather always start
  at the folder picker. `--port`/`-p` is also accepted here if you don't want
  to set `port` in the config.
- `ProtectSystem=strict` makes essentially the whole filesystem read-only to
  the process except `/dev`, `/proc`, `/sys`, and whatever you explicitly
  list in `ReadWritePaths` — which must therefore include both
  `WorkingDirectory` (so `~/.wede/` — i.e. `/srv/wede/.wede/` — can be
  written: sessions, tokens, lockout, trust, tunnel state) and whatever tree
  `workspace_root` points at.
- `ProtectHome=read-only` additionally locks down `/home`, `/root`, and
  `/run/user/*`. It's inert here since the service user's home (`/srv/wede`)
  isn't under any of those paths, but it costs nothing and blocks the shared
  terminal from writing into a real person's home directory if this same
  wede instance is ever pointed at a workspace that happens to live there.

Enable and start it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now wede
sudo systemctl status wede
sudo journalctl -u wede -f
```

A healthy start logs a line like `wede vX.Y.Z running on http://127.0.0.1:9090`
followed by either `workspace: /srv/projects` or `no default workspace - open
a folder from the UI` (see `main.go`).

### Hardening you should NOT add here

The directives above are chosen deliberately narrow. **Do not** reach for
systemd's stronger sandboxing knobs without understanding that wede's shared
terminal is, by design, an unrestricted shell for anyone with an editor link:

- **`PrivateNetwork=true`** breaks git push/pull/fetch, the built-in API
  client, and the Ephor public tunnel (all real outbound network use from
  inside the wede process or its terminal children).
- **A tight `SystemCallFilter`** (beyond the default-safe `@system-service`
  set) or **`RestrictAddressFamilies`** narrower than `AF_UNIX AF_INET
  AF_INET6` will start breaking arbitrary commands editors run in the shared
  terminal (`ssh`, `docker`, language toolchains, `sudo` if you ever allow
  it) — you cannot syscall-filter a general-purpose login shell without also
  restricting what people can do *in* it.
- **`DynamicUser=true`** conflicts with wanting a stable, inspectable home
  directory for `~/.wede/` state and for workspace ownership.

If your actual goal is to stop an editor collaborator from having full host
access, systemd hardening is the wrong tool — the fix is not sharing editor
links with people you wouldn't give a shell account to, per the
[security model](ARCHITECTURE.md#no-sandbox-warning-for-shared-deployments).
Viewer-role links do not get a terminal, LSP, or DAP socket at all
(`RequireEditor`-gated in `backend/cmd/wede/main.go`) and are safe to hand out
more freely.

---

## 3. launchd (macOS)

For a Mac acting as a home server (or just wanting wede to survive a reboot
without a login shell open), create a launchd agent. There's no user-isolation
story here comparable to a dedicated Linux system user — launchd agents run as
whichever account owns them — so the same "don't hand out editor links you
wouldn't give a shell to" rule applies to your own Mac account.

`~/Library/LaunchAgents/org.vulos.wede.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>org.vulos.wede</string>

    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/wede</string>
        <string>/Users/you/projects</string>
    </array>

    <key>WorkingDirectory</key>
    <string>/Users/you</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>WEDE_WORKSPACE_ROOT</key>
        <string>/Users/you/projects</string>
    </dict>

    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>/Users/you/Library/Logs/wede.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/you/Library/Logs/wede.error.log</string>
</dict>
</plist>
```

Load and start it:

```bash
launchctl load -w ~/Library/LaunchAgents/org.vulos.wede.plist
launchctl start org.vulos.wede
tail -f ~/Library/Logs/wede.log
```

launchd has no `journalctl`; the `StandardOutPath`/`StandardErrorPath` files
above are where wede's `log.Printf` output goes instead. `KeepAlive=true`
gives you the equivalent of systemd's `Restart=on-failure`.

---

## 4. Windows

wede's release pipeline (`.github/workflows/release.yml`) cross-compiles a
`wede-windows-amd64.exe` asset for every tagged release, so the binary itself
runs on Windows x86_64 — there is no `windows/arm64` build. Be aware of one
real gap before you rely on it for collaborative use:

**The shared terminal spawns `$SHELL -l`** (`backend/internal/terminal/terminal.go`),
falling back to `cmd.exe` if the `SHELL` environment variable isn't set on
Windows. `cmd.exe` doesn't understand a `-l` argument the way a POSIX login
shell does — this code path is written for `bash`/`zsh`/`sh`-style shells and
isn't Windows-native. If you want the terminal feature to behave well on
Windows, set `SHELL` to a real shell before launching wede — Git Bash's
`bash.exe`, or a WSL shell — rather than relying on the bare `cmd.exe`
fallback. Everything else (file editing, git, search, LSP/DAP if the
binaries are on `PATH`) doesn't go through this code path and is unaffected.

wede ships no native Windows service wrapper. To run it unattended:

- **Task Scheduler** — create a task triggered "At startup" (or "At log on"),
  action = start `wede.exe` with your workspace path as an argument, and set
  it to run whether or not a user is logged on if you want it to survive
  logout. Config resolution is the same search order as everywhere else
  (`wede.config.json` next to the `.exe`, or under `%USERPROFILE%\.config\wede\`).
- **NSSM** (the Non-Sucking Service Manager) — wraps `wede.exe` as a proper
  Windows service (`nssm install wede C:\path\to\wede.exe C:\path\to\project`),
  giving you start/stop/restart semantics and Windows Event Log output
  without writing your own wrapper.

If your primary use case is Linux-shaped tooling (git, language servers,
POSIX shells) on a Windows box, running wede under **WSL2** as a normal Linux
binary is the best-tested path — everything in the Linux/systemd section
above applies unchanged inside the WSL distro.

---

## 5. State & backup

wede has no database — every persistent thing it keeps is a small JSON file
(or, for chat/requests, plain text/JSON files that are meant to be readable
and committable). Two locations matter: `~/.wede/` (the *server's* home
directory — the OS user wede runs as) and `<workspace>/.wede/` (per project,
and relocatable — see below).

### `~/.wede/` — server-wide state

Created by `auth.New` at startup with mode `0700`. All files inside are
`0600` unless noted.

| File | What it is | Secret? | Back up? | If you delete it |
|---|---|---|---|---|
| `sessions.json` | Active session tokens, keyed by **SHA-256 hash** of the token (`auth.go`) — created-at timestamp, chosen username, role. | No usable secret (hash, not the raw token) — but metadata. | Not important. | Everyone currently logged in is signed out; they log in / redeem their invite link again. |
| `tokens.json` | Minted share tokens: SHA-256 hash of the raw token, role, optional username/expiry (`tokens.go`). | No usable secret (hash) — but reveals how many links exist and their roles. | Optional. | Every outstanding share link stops working. Mint new ones from **Settings → Share links**. |
| `lockout.json` | Failed-login attempt counter + locked flag (`auth.go`). | No. | No. | Resets the brute-force lockout — see [TROUBLESHOOTING.md](TROUBLESHOOTING.md#locked-out-after-failed-logins) for the exact recovery steps (a file delete alone is not enough; the process must restart). |
| `trusted.json` | Absolute paths of workspace roots the owner has trusted to run their committed `.wede/` tooling config (`trust/trust.go`). | No secret, but leaks workspace path names. | Optional. | You'll be prompted to re-trust any workspace that ships `.wede/tasks.json`, `formatters.json`, or `debug.json` before those run again. |
| `tunnel.json` | Ephor public-tunnel config: relay `serverUrl`, public `name`, and the relay **bearer token** (`tunnel/tunnel.go`). | **Yes — the `token` field is a real credential**, stored in plaintext. | **Yes, if you use the tunnel.** Losing it means re-entering the relay token in **Settings → Public access**. | The tunnel stops on next start; nothing else is affected. |
| `wede-hosts.json` | Map of workspace root → the workspace-relative folder that physically hosts that workspace's `.wede/` directory, for workspaces where you've relocated it (`workspace/wedehost.go`). | No. | Only if you've used "relocate `.wede`". | wede falls back to assuming `.wede` lives at the workspace root — if you'd actually moved it elsewhere, wede won't find the moved directory until you re-set the location from **Settings**. |
| `lsp.json`, `formatters.json`, `tasks.json`, `debug.json` | Your own optional global tool registries (LSP servers, formatters, tasks, debug adapters). wede only *reads* these — it never writes or manages their permissions. | Depends on contents (usually just command names). | Yes, if you've customized them — they're hand-authored and not regenerated. | The corresponding built-ins-only defaults apply. |

### `<workspace>/.wede/` — per-project state

By default this lives at the workspace root; the owner can relocate it into
any top-level subfolder via **Settings → wede location** (`PUT
/api/workspaces/{id}/wede-location`), which *moves* the existing directory
(`workspace/wedehost.go`). All the paths below are relative to wherever
`.wede` currently lives for that workspace, not necessarily the workspace
root.

| Path | What it is | Secret? | Back up? | If you delete it |
|---|---|---|---|---|
| `chat.md` | Public channel history, markdown, git-activity messages included (`chat/chat.go`). Meant to be committed — any collaborator or an LLM working on the repo can read it. | No. | Only if the conversation itself matters to you; it's ordinary repo content. | Public chat history is gone (git-activity messages are re-derivable from `git log`; typed messages are not). |
| `private/chat.md` + `private/.gitignore` entry | Private channel history. wede writes a `private/` line into `.wede/.gitignore` the first time this channel is used, so it's excluded from commits by default. | Contents-dependent — whatever your team discussed privately. | Your call. | Private chat history is gone; the channel starts empty on next use. |
| `requests/` | Saved API-client requests (JSON), meant to be committed alongside the project (`apiclient.go`). | Usually no, unless a request body hardcodes a credential. | Yes, if you rely on them — they're ordinary project files. | Saved requests disappear from the API client panel. |
| `environments/` | API-client environments (`{{variable}}` sets) for the requests above. | **Can be, if you've put real API keys/tokens in variable values** — these are plain JSON meant to be committed by default, so don't paste secrets in here unless you also `.gitignore` them yourself. | Same caveat as above. | Saved environments disappear. |
| `tasks.json`, `formatters.json`, `debug.json` | Project-committed tool config (build/test commands, formatters, debug adapters). Only takes effect after the **owner** trusts the workspace (`trust/trust.go`) — this is a deliberate gate, since these files define host commands. | No secret, but defines executable behavior. | Yes — these are meant to be committed, like the rest of the repo. | Falls back to the owner's global `~/.wede/` equivalents (if any) plus wede's own built-ins. |

### Collaborative editing (CRDT) state: there isn't a separate copy

This is worth stating plainly because it's easy to assume otherwise: **wede
does not keep a shadow copy of your files for the CRDT layer.**
`internal/collabdoc/persistence.go`'s `DiskPersistence` debounces live edits
(600 ms after the last keystroke) and writes the *materialized plain text*
straight back to the real file at its normal path, atomically (temp file +
rename). There is no `.wede/` cache of document state to back up or worry
about going stale — back up the workspace the same way you'd back up any
other directory (and normally, via git).

---

## 6. Upgrading

Standard in-place sequence:

```bash
sudo systemctl stop wede          # (or: launchctl stop org.vulos.wede)
sudo cp /usr/local/bin/wede /usr/local/bin/wede.bak   # optional, cheap insurance
sudo cp ./wede-linux-amd64 /usr/local/bin/wede        # replace the binary
sudo chmod +x /usr/local/bin/wede
sudo systemctl start wede
sudo journalctl -u wede -f        # confirm a clean "wede vX.Y.Z running on ..." line
```

**Back up before you upgrade, at minimum:**

- `wede.config.json` — contains your password in plaintext.
- `~/.wede/tunnel.json` — contains your relay bearer token in plaintext, if
  you use the public tunnel.

Everything else in `~/.wede/` (sessions, tokens, lockout, trust, wede-hosts)
is non-critical: worst case after a loss is re-logging-in, re-minting share
links, or re-trusting a workspace, none of which lose project data.

**On version compatibility — an honest statement, not a reassurance:**
there is no documented schema-versioning or migration mechanism for
`~/.wede/`'s JSON state files across releases. The one concrete backward-compat
shim that does exist in the code is for the session role field: a session
record with no stored `Role` (from before roles existed) is treated as
`owner` (`auth.go`) — evidence the project *has* handled at least one schema
change gracefully, but not a blanket guarantee that every future change will
be. What **is** guaranteed is that `wede.config.json` parsing is strict
(`json.Decoder.DisallowUnknownFields()`): if a future version renames or
removes a config key you have set, startup **fails loudly** with `invalid
wede.config.json: json: unknown field "..."` rather than silently ignoring
your setting — check `journalctl -u wede` for exactly that message first if
wede won't start after an upgrade. For the JSON state files in `~/.wede/`
and `<workspace>/.wede/`, the code does a best-effort `json.Unmarshal` and
tolerates missing/extra fields; there's no equivalent hard failure or
migration step documented, so if you're upgrading across a large version gap
and something looks wrong (a lost session, a tool config not applying), check
the [CHANGELOG](../CHANGELOG.md) for that release rather than assuming a bug.

---

## 7. Resource expectations

wede has no published benchmark numbers — the notes below are about *what
drives* resource use, not measured figures, so treat any concrete number you
see elsewhere as approximate.

- **Base process:** one Go binary, one `net/http` server, the entire frontend
  served from memory via `go:embed` (no disk I/O per request for the SPA).
  Idle footprint is small and mostly independent of project size, since
  files are read on demand rather than indexed up front.
- **Per terminal, one PTY child process** (`internal/terminal/terminal.go`):
  each shared terminal session keeps a real shell process alive for as long
  as the session exists (it survives WebSocket reconnects), plus a fixed
  64 KB in-memory scrollback ring buffer per session for replay to
  reconnecting/late-joining viewers. Cost scales with how many terminals are
  open, not how many people are watching them (a terminal is shared, not
  duplicated, per viewer).
- **Per (workspace, language) pair, one LSP server child process**
  (`internal/lsp/lsp.go`) — **this is the real memory cost**, not wede
  itself. `gopls` and `rust-analyzer` in particular can index an entire
  project into memory; how much depends entirely on the language server and
  the size of the project it's pointed at, which is outside wede's control.
  Servers are cached per workspace+language and reused across reconnects;
  they're killed when the workspace changes.
- **Per active debug session, one DAP adapter child process**
  (`internal/dap/dap.go`) — unlike LSP, there's no caching here: a fresh
  adapter process is spawned per debug session and killed when the socket
  closes.
- **Search:** with `rg` on `PATH`, search runs as a short-lived external
  process per query. Without it, the pure-Go fallback walks the tree
  in-process. Either way, results are capped (500 matches; replace-across-files
  capped at 200 files / 10,000 replacements — `internal/search/search.go`),
  which bounds worst-case work per request regardless of repo size.
- **File watching:** one `fsnotify` watcher per open workspace, debounced
  250 ms before it pushes an SSE event — negligible on its own, but it's
  one more open file-descriptor set per workspace.

Practical takeaway: for sizing a box, budget for **N terminals × a shell
process each**, plus **one language server per language you actually use per
open workspace** (and size that against what `gopls`/`rust-analyzer`/etc.
need for *your* project, not wede) — wede's own process overhead is the
smallest line item.

---

## See also

- [PUBLIC-ACCESS.md](PUBLIC-ACCESS.md) — TLS, reverse proxies, and tunnel
  options for reaching wede from outside `localhost`.
- [CONFIGURATION.md](CONFIGURATION.md) — every config key, in full.
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — symptom → cause → fix for the
  problems people actually hit running wede day to day.
