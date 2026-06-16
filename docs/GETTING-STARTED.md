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

By default wede binds to `127.0.0.1` (localhost only). To access it from another machine:

```json
{
  "password": "your-strong-password-here",
  "port": "9090",
  "host": "0.0.0.0"
}
```

> **Warning:** Exposing wede on `0.0.0.0` means it is reachable on all network interfaces. Use a strong password and consider placing it behind a reverse proxy with TLS.

---

## Embedding in Vulos OS

wede integrates with the Vulos OS app shell via `frame_ancestors`. See [CONFIGURATION.md](CONFIGURATION.md#embedding-in-an-iframe) for details.

---

## Next steps

- [CONFIGURATION.md](CONFIGURATION.md) — full config reference
- [ARCHITECTURE.md](ARCHITECTURE.md) — how wede is built internally
- [SCREENSHOTS.md](SCREENSHOTS.md) — visual tour of the IDE
- [../ROADMAP.md](../ROADMAP.md) — what's coming next
