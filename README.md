<div align="center">

<img src="public/icon.svg" alt="wede — self-hosted web IDE" width="80" height="80">

# wede

**A lightweight, open-source, self-hosted web IDE.**<br>
**Code editor, terminal, git, and file explorer — all in your browser.**

One ~10MB binary. No cloud, no Docker, no subscriptions.<br>
Deploy on your server, NAS, Raspberry Pi, or run locally.

[![Build](https://img.shields.io/github/actions/workflow/status/webcrft/wede/build.yml?branch=main&style=flat-square)](https://github.com/webcrft/wede/actions)
[![Release](https://img.shields.io/github/v/release/webcrft/wede?style=flat-square)](https://github.com/webcrft/wede/releases)
[![License](https://img.shields.io/github/license/webcrft/wede?style=flat-square)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.22+-00ADD8?style=flat-square&logo=go)](https://go.dev)

[Website](https://wede.pty.it.com/) · [Install](#quick-install) · [Screenshots](#screenshots) · [Docs](#getting-started)

</div>

<br>

<div align="center">
<img src="docs/screenshots/full_light.png" alt="wede self-hosted web IDE running in browser with code editor, terminal, and git integration" width="800">
<br>
<em>wede — browser-based IDE with light mode (Daylight theme)</em>
</div>

<br>

## Features

| | Feature | Description |
|---|---|---|
| :file_folder: | **File Explorer** | VS Code-style project tree with git status colors (green, yellow, red). Context menu for copy, paste, rename, delete. |
| :pencil2: | **Code Editor** | CodeMirror 6 with syntax highlighting for JavaScript, TypeScript, Go, Python, Rust, and 10+ languages. Dark/light themes. |
| :computer: | **Web Terminal** | Full PTY terminal emulator via xterm.js and WebSocket. Multiple tabs. Run shell commands, SSH, Docker — anything. |
| :electric_plug: | **Git Client** | Built-in visual commit graph, staging area, branch management, and checkout. Right-click context menus on commits. |
| :globe_with_meridians: | **Built-in Browser** | Preview your running web app in an embedded browser tab without leaving the IDE. |
| :iphone: | **Mobile Friendly** | Fully responsive UI for tablets and phones. Edit code and run commands from iPad or Android. |
| :crescent_moon: | **Dark & Light Themes** | Midnight (dark) and Daylight (light) color schemes. Theme-aware terminal and editor. |
| :lock: | **Secure Access** | Password authentication with 3-attempt lockout. Deploy behind HTTPS reverse proxy for production. |

## Screenshots

<table>
<tr>
<td><img src="docs/screenshots/git_graph.png" alt="wede visual git commit graph" width="400"><br><em>Git commit graph</em></td>
<td><img src="docs/screenshots/preview.png" alt="wede built-in browser preview" width="400"><br><em>Built-in browser preview</em></td>
</tr>
</table>

## Why wede?

- **No cloud dependency** — your code never leaves your machine. No GitHub Codespaces, no Gitpod, no monthly bill.
- **Single binary** — one ~10MB Go executable with the frontend embedded. No Docker, no Node.js runtime, no database.
- **Run anywhere** — Linux servers, macOS, Raspberry Pi, NAS devices, air-gapped networks, CI runners.
- **Access from any device** — code from your laptop, tablet, or phone through any modern browser.
- **Self-hosted alternative** to code-server, VS Code Server, Theia, and cloud IDEs.

## Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/webcrft/wede/main/install.sh | bash
```

Or download the binary directly from [GitHub Releases](https://github.com/webcrft/wede/releases).

## Getting Started

**1. Create a config file** in your project directory:

```json
{
  "password": "your-password",
  "port": "9090"
}
```

Save this as `wede.config.json`.

**2. Start wede:**

```bash
wede /path/to/your/project
```

**3. Open your browser** at [http://localhost:9090](http://localhost:9090) and log in with your password.

## CLI Usage

```
wede [flags] [path]
```

| Flag | Description | Default |
|------|-------------|---------|
| `path` | Project directory to open | _(none — shows folder picker)_ |
| `--port` | Override listen port | From config or `9090` |

wede looks for `wede.config.json` in the current directory or parent directories.

## Configuration

wede is configured via a `wede.config.json` file:

```json
{
  "password": "your-password",
  "port": "9090"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `password` | `string` | Password for browser login. Required. |
| `port` | `string` | Port to listen on. Default: `9090`. |

## Development

Run the frontend and backend separately for development with hot reload:

**Frontend** (React + Vite):

```bash
npm install
npm run dev
```

**Backend** (Go):

```bash
cd backend
go run ./cmd/wede .
```

The Vite dev server proxies API and WebSocket requests to the Go backend.

**Build a single binary:**

```bash
npm run build:all
```

This builds the frontend, embeds it in the Go binary, and outputs `./wede`.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | [Go](https://go.dev) |
| Frontend | [React 19](https://react.dev) + [Vite](https://vite.dev) |
| Styling | [Tailwind CSS 4](https://tailwindcss.com) |
| Editor | [CodeMirror 6](https://codemirror.net) |
| Terminal | [xterm.js](https://xtermjs.org) |
| Icons | [Lucide](https://lucide.dev) |
| Fonts | Space Grotesk, Inter, JetBrains Mono |

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes
4. Push to the branch (`git push origin feature/my-feature`)
5. Open a pull request

## Author

Vibe coded by <a href="https://github.com/imranparuk"><img src="https://github.githubassets.com/favicons/favicon-dark.svg" width="14" height="14" alt="GitHub"> <strong>imranparuk</strong></a>

## License

[MIT](LICENSE)

---

<div align="center">

<a href="https://wede.pty.it.com">Website</a> · <a href="https://github.com/webcrft/wede/issues">Issues</a> · <a href="https://github.com/webcrft/wede/releases">Releases</a>

<br>

<sub>wede is a free, open-source, self-hosted web IDE and remote development environment.<br>
Built as an alternative to code-server, VS Code Server, Gitpod, and GitHub Codespaces.<br>
Keywords: web IDE, self-hosted IDE, browser code editor, remote development, online terminal,<br>
git client, open source IDE, developer tools, Go web server, single binary IDE.</sub>

</div>
