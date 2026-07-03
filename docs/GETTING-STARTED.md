# Getting Started with wede

wede is a single-binary, self-hosted web IDE. This guide walks you from zero to a running instance.

---

## Prerequisites

- A machine running Linux, macOS, or Windows
- A modern browser (Chrome, Firefox, Safari, Edge)
- No Docker, no database, no Node.js runtime required at runtime

---

## Installation

### One-liner installer (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/vul-os/wede/main/install.sh | bash
```

The installer downloads the binary for your platform, generates a random password, writes `wede.config.json`, and prints your password. **Note it down** — it will not be shown again.

### Manual download

Download the latest binary from [GitHub Releases](https://github.com/vul-os/wede/releases) for your platform:

| Platform | File |
|----------|------|
| Linux x86_64 | `wede-linux-amd64` |
| Linux ARM64 | `wede-linux-arm64` |
| macOS x86_64 | `wede-darwin-amd64` |
| macOS ARM64 (Apple Silicon) | `wede-darwin-arm64` |
| Windows x86_64 | `wede-windows-amd64.exe` |

Make it executable and place it in your `$PATH`:

```bash
chmod +x wede-linux-amd64
mv wede-linux-amd64 /usr/local/bin/wede
```

---

## Configuration

Create `wede.config.json` in your project directory (or any parent directory):

```json
{
  "password": "your-strong-password-here",
  "port": "9090"
}
```

> **Security:** The file is gitignored by default. Never commit a config file containing a real password.

See [CONFIGURATION.md](CONFIGURATION.md) for the full reference.

---

## Starting wede

```bash
# Open a specific project directory
wede /path/to/your/project

# Start without a workspace (folder picker shown in UI)
wede

# Override port
wede --port 8080 /path/to/project
```

Open [http://localhost:9090](http://localhost:9090) in your browser and log in with your password.

---

## First steps in the IDE

1. **File Explorer** (left sidebar) — browse and manage your project files. Right-click for context menu options.
2. **Editor** — click any file to open it. 25+ languages with syntax highlighting, auto-save, and LSP diagnostics when language servers are installed (see [Adding language support](#adding-language-support)).
3. **Terminal** (bottom panel, or `Ctrl+` `` ` ``) — full PTY terminal. Multiple tabs supported.
4. **Git panel** (sidebar) — stage changes, write commit messages, push/pull, view the commit graph.
5. **Search** (`Ctrl+Shift+F`) — workspace-wide search with ripgrep. Supports regex and replace-across-files.
6. **Command palette** (`Ctrl+Shift+P`) — fuzzy-search all IDE commands.

---

## Adding language support

wede gets code intelligence (diagnostics, hover, completion, go-to-definition)
the same way VS Code does — through **Language Server Protocol** servers. It can
use **any LSP server**, not just the built-in ones, with no recompiling.

**Built-in** (auto-detected on your `PATH`): `gopls` (Go),
`typescript-language-server` (JS/TS), `pylsp` (Python), `rust-analyzer` (Rust).
Install the binary and wede picks it up — check **Settings → Language server**
for what it found.

**Add your own** — create `~/.wede/lsp.json`:

```json
{
  "servers": {
    "c":     { "command": "clangd",                "extensions": ["c", "h"] },
    "cpp":   { "command": "clangd",                "extensions": ["cpp", "cc", "hpp"] },
    "lua":   { "command": "lua-language-server",    "extensions": ["lua"] },
    "ruby":  { "command": "solargraph", "args": ["stdio"], "extensions": ["rb"] },
    "bash":  { "command": "bash-language-server", "args": ["start"], "extensions": ["sh", "bash"] }
  }
}
```

Each entry maps a language to its server `command` (located on `PATH`, like the
built-ins), optional `args`, and the file `extensions` that should use it. An
entry with a name matching a built-in (e.g. `go`) overrides it. Restart wede to
apply. The command must be installed and on your `PATH`.

> Syntax highlighting is independent of LSP and already covers 25+ languages out
> of the box; LSP adds the *intelligence* on top.

### Format on save for any language

Built-in formatters cover Go (`gofmt`), JS/TS/CSS/JSON/HTML/Markdown
(`prettier`), and Python (`black`). Add a formatter for any other language in
`~/.wede/formatters.json` — the command receives the source on **stdin** and must
write the result to **stdout** (`{file}` in args is replaced with the file name):

```json
{
  "formatters": {
    "rs":   { "command": "rustfmt" },
    "lua":  { "command": "stylua", "args": ["-"] },
    "sh":   { "command": "shfmt",  "args": ["-"] },
    "swift":{ "command": "swift-format" }
  }
}
```

Enable **Settings → Format on save** (or `Ctrl/Cmd+S`). User formatters override
the built-ins for the same extension. The global `~/.wede/formatters.json` always
applies; a project may also commit `<workspace>/.wede/formatters.json`, which is
used **only after you trust the workspace** (see [Workspace trust](#workspace-trust)).

### Tasks — named build/test/run commands

Define commands in `~/.wede/tasks.json`; they appear in a **Tasks** panel in the
activity rail and run in a terminal:

```json
{
  "tasks": [
    { "name": "Build",  "command": "go build ./..." },
    { "name": "Test",   "command": "go test ./...", "cwd": "backend" },
    { "name": "Dev",    "command": "npm run dev" }
  ]
}
```

Click a task (or pick it from the panel) and wede opens a new terminal tab named
after it and runs the command — `cwd` (relative to the workspace) is optional.
Running uses the terminal, so it's editor-gated (viewers can see tasks but not
run them). The global `~/.wede/tasks.json` always applies; a project may also
commit `<workspace>/.wede/tasks.json`, used **only after you trust the workspace**.

### Workspace trust

LSP servers, formatters, and tasks all run **commands on the host**. The owner's
global `~/.wede/` config is always trusted, but a project's *committed*
`.wede/formatters.json` / `.wede/tasks.json` could otherwise let any editor run
code as the owner. So committed tool config is **ignored until the owner trusts
the workspace** — toggle **Settings → Workspace trust** (owner-only). Untrusting
revokes it immediately. Only trust workspaces whose collaborators you trust.

> The trusted set is stored in `~/.wede/trusted.json`. The currently-bundled LSP
> registry is global-only; project `.wede/lsp.json` support lands with the same
> trust gate.

### Debugging (DAP)

wede speaks the **Debug Adapter Protocol**, the same standard VS Code uses, so it
can drive any debug adapter. Built-in: **`dlv`** (Go, `dlv dap`) and **`debugpy`**
(Python); install the adapter binary and it's auto-detected. Add more in
`~/.wede/debug.json` (or a trusted project `.wede/debug.json`):

```json
{ "adapters": { "node": { "command": "js-debug-adapter", "extensions": ["js", "ts"] } } }
```

Open a debuggable file, click the gutter to set **breakpoints**, then **Run &
Debug** (activity rail) → **Start Debugging**. You get stepping (continue / step
over / into / out), the **call stack**, **variables**, and a debug console; the
current line is highlighted as you step. Debugging runs code, so it's editor-gated.

VS Code `.vsix` extensions are **not** supported — they require a Node extension
host, which is intentionally outside wede's single-binary design. LSP and DAP are
the portable, editor-agnostic equivalents and cover the language + debugging use cases.

---

## Exposing wede over a network

By default wede binds to `127.0.0.1` (localhost only) — reachable only from the
machine it runs on. There are three ways to reach it from elsewhere, in
increasing order of reach.

### 1. Same LAN — bind to all interfaces

To reach wede from another device on the same network:

```json
{
  "password": "your-strong-password-here",
  "port": "9090",
  "host": "0.0.0.0"
}
```

> **Warning:** Exposing wede on `0.0.0.0` means it is reachable on all network
> interfaces. Use a strong password and consider placing it behind a reverse
> proxy with TLS. This does **not** make wede reachable from outside your LAN.

### 2. Through Vulos — no public exposure

If you run [Vulos OS](https://vulos.org), you don't need to expose a port at all.
wede runs as a first-class app inside the Vulos shell, and the Vulos gateway
handles routing and authentication for you — you reach wede through your Vulos
instance like any other app. Keep wede bound to loopback and set
`frame_ancestors` to your Vulos origin so it can embed:

```json
{
  "password": "your-strong-password-here",
  "port": "9090",
  "host": "127.0.0.1",
  "frame_ancestors": "https://vulos.org"
}
```

See [Embedding in Vulos OS](#embedding-in-vulos-os) below and
[CONFIGURATION.md](CONFIGURATION.md#embedding-in-an-iframe) for details.

### 3. Public internet — a sovereign Vulos Relay

To reach wede from anywhere without opening ports on your home network, put it
behind your **own** [Vulos Relay](https://github.com/vul-os/vulos-relay) server —
a small sovereign reverse-tunnel relay you run on a cheap VPS with a public IP.
wede embeds the relay **agent** in-process: there is no third-party `frp` binary
to install. The agent dials a single outbound `wss://` connection to your relay,
so the wede machine needs no inbound ports or static IP.

**On the VPS** — run the Vulos Relay server (see the vulos-relay repo for the
build/deploy details) with a public HTTPS endpoint, e.g. `relay.example.com`, and
mint a bearer token authorizing your chosen public name (`wede`). Point a DNS
record for the relay (and the name's subdomain, if your relay uses vhost routing)
at the VPS.

**In wede** — no config files. Open **Settings → Public access (Vulos Relay)** as
the owner and fill in:

| Field | Example | Notes |
|-------|---------|-------|
| relay server URL | `wss://relay.example.com` | `http`/`https`/`ws`/`wss` all accepted |
| relay token | `a-long-random-secret` | the token your relay minted; authorizes the name |
| public name | `wede` | must be authorized by the token |

Click **Start tunnel**. wede's agent dials the relay, claims the name, and the
panel shows the live public URL — mapped straight to wede's loopback port on your
machine. The config is persisted to `~/.wede/tunnel.json` (token file-mode `0600`,
and always redacted when read back over the API).

> **Security:** A public tunnel means anyone who finds the URL hits your login
> page. Use a strong wede `password` and a long random relay token, and terminate
> TLS at the relay (`wss://`). The agent only ever proxies to wede's own loopback
> port — it refuses any non-loopback target (SSRF guard) — and dials outbound
> only, so your home network stays closed.

---

## Embedding in Vulos OS

wede integrates with the Vulos OS app shell via `frame_ancestors`. See [CONFIGURATION.md](CONFIGURATION.md#embedding-in-an-iframe) for details.

---

## Next steps

- [CONFIGURATION.md](CONFIGURATION.md) — full config reference
- [ARCHITECTURE.md](ARCHITECTURE.md) — how wede is built internally
- [SCREENSHOTS.md](SCREENSHOTS.md) — visual tour of the IDE
- [../ROADMAP.md](../ROADMAP.md) — what's coming next
