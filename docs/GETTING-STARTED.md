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
2. **Editor** — click any file to open it. Supports 12+ languages with syntax highlighting, auto-save, and LSP diagnostics when language servers are installed.
3. **Terminal** (bottom panel, or `Ctrl+` `` ` ``) — full PTY terminal. Multiple tabs supported.
4. **Git panel** (sidebar) — stage changes, write commit messages, push/pull, view the commit graph.
5. **Search** (`Ctrl+Shift+F`) — workspace-wide search with ripgrep. Supports regex and replace-across-files.
6. **Command palette** (`Ctrl+Shift+P`) — fuzzy-search all IDE commands.

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

### 3. Public internet — a VPS with frp

To reach wede from anywhere without opening ports on your home network, put it
behind a small public relay using [frp](https://github.com/fatedier/frp) (Fast
Reverse Proxy, MIT-licensed). You run the frp **server** (`frps`) on a cheap VPS
with a public IP, and the frp **client** (`frpc`) next to wede. The client dials
out to the server, so the wede machine needs no inbound ports or static IP.

**On the VPS** — `frps.toml`:

```toml
bindPort     = 7000
auth.token   = "a-long-random-shared-secret"
vhostHTTPPort = 80
```

```bash
./frps -c frps.toml
```

**On the wede machine** — `frpc.toml`:

```toml
serverAddr = "your.vps.public.ip"
serverPort = 7000
auth.token = "a-long-random-shared-secret"   # must match frps

[[proxies]]
name          = "wede"
type          = "http"
localPort     = 9090                          # wede's port
customDomains = ["wede.example.com"]          # DNS A-record -> your VPS
```

```bash
./frpc -c frpc.toml
```

wede is now reachable at `http://wede.example.com`, mapped straight to
`localhost:9090` on your machine. No domain? Use a raw TCP tunnel instead —
set `type = "tcp"` and `remotePort = 9090`, then browse to
`http://your.vps.public.ip:9090`.

> **Security:** A public tunnel means anyone who finds the URL hits your login
> page. Use a strong wede `password`, a long random frp `auth.token`, and
> terminate TLS at the VPS (frp supports HTTPS vhosts, or front `frps` with
> Caddy/nginx for automatic certificates). Run multiple wede instances or other
> local services by adding more `[[proxies]]` blocks.

---

## Embedding in Vulos OS

wede integrates with the Vulos OS app shell via `frame_ancestors`. See [CONFIGURATION.md](CONFIGURATION.md#embedding-in-an-iframe) for details.

---

## Next steps

- [CONFIGURATION.md](CONFIGURATION.md) — full config reference
- [ARCHITECTURE.md](ARCHITECTURE.md) — how wede is built internally
- [SCREENSHOTS.md](SCREENSHOTS.md) — visual tour of the IDE
- [../ROADMAP.md](../ROADMAP.md) — what's coming next
