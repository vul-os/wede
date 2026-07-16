<div align="center">

<img src="public/icon.svg" alt="wede" width="80" height="80">

# WEDE

<sub>**WEb IDE**</sub>

***Putting the WE in WEb IDE.***<br>
**A self-hosted, collaborative web IDE in a single Go binary.**<br>
**Real-time multi-user editing, shared terminals, VS Code-grade git, and chat — all in your browser.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/vul-os/wede?style=flat-square)](https://github.com/vul-os/wede/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/vul-os/wede/ci.yml?branch=main&style=flat-square)](https://github.com/vul-os/wede/actions)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)](https://react.dev)

*Vulos — rooted in **vula**, the Zulu and Xhosa word for **open**.*

![wede IDE](docs/screenshots/hero.png)

</div>

---

> **Status: deprioritized.**
> wede is **not under active development** and is effectively
> community-maintained. It has not been deprecated or removed — but expect no
> roadmap work, and **use it at your own risk**. If you depend on it, build from
> source (see [Quick start](#quick-start)), pin a commit, and be prepared to
> maintain your own fork.

---

## Overview

wede is a single ~10 MB Go binary that serves a full **collaborative** web IDE straight from your machine. No cloud dependency, no Docker, no subscriptions, no database. Deploy it on a server, a NAS, a Raspberry Pi, or just run it locally — then code from any device through your browser, alone or with your whole team.

One host serves many people: open multiple projects as **workspaces**, invite others with **share links**, edit the same files with **multiplayer cursors**, share the same **terminals**, and talk in per-workspace **chat** — all with no accounts and no external services. It runs standalone or embedded as a first-class app in the [Vulos OS](https://vulos.org) shell via `frame_ancestors` iframe integration.

[GitHub](https://github.com/vul-os/wede) · [Quick start](#quick-start) · [Docs](#documentation) · [Changelog](CHANGELOG.md) · [Roadmap](ROADMAP.md)

---

## Deployment modes

wede is a single self-hosted binary that runs three ways — every shape on
hardware you control:

- **Standalone (self-host)** — run `./wede /path/to/project` on a laptop, server, NAS, or Pi; loopback by default, LAN with `"host": "0.0.0.0"`. This is the primary shape.
- **Embedded behind an iframe host** — set `frame_ancestors` (e.g. `https://vulos.org`) so a host shell can embed wede as an app tile behind its own routing and auth; the same binary either way.
- **Public exposure** — expose a loopback-bound wede over *your own* [Vulos Relay](https://github.com/vul-os/vulos-relay) server via the embedded relay agent (no inbound ports, no static IP).

Details in [Remote access](#remote-access) and [docs/GETTING-STARTED.md](docs/GETTING-STARTED.md).

---

## Screenshots

<table>
<tr>
<td><img src="docs/screenshots/hero.png" alt="wede IDE — editor and file tree" width="400"><br><em>IDE main view — editor + file tree</em></td>
<td><img src="docs/screenshots/git.png" alt="wede git panel with diff" width="400"><br><em>Git panel — staging with inline diff</em></td>
</tr>
<tr>
<td><img src="docs/screenshots/git_graph.png" alt="wede visual git commit graph with branches and merges" width="400"><br><em>Git commit graph — branches, merges &amp; refs</em></td>
<td><img src="docs/screenshots/apiclient.png" alt="wede built-in Postman-style API client" width="400"><br><em>Built-in API client (Postman-style)</em></td>
</tr>
<tr>
<td><img src="docs/screenshots/terminals_floating.png" alt="wede movable floating terminal windows" width="400"><br><em>Terminals as movable windows (synced across collaborators)</em></td>
<td><img src="docs/screenshots/chat.png" alt="wede live workspace chat with public and private channels" width="400"><br><em>Live chat — public &amp; private channels</em></td>
</tr>
<tr>
<td><img src="docs/screenshots/search.png" alt="wede workspace search panel" width="400"><br><em>Comprehensive search (regex, globs, replace)</em></td>
<td><img src="docs/screenshots/settings.png" alt="wede settings panel" width="400"><br><em>Settings — editor, LSP, themes, tunnel</em></td>
</tr>
<tr>
<td><img src="docs/screenshots/command_palette.png" alt="wede command palette" width="400"><br><em>Command palette (Ctrl+Shift+P)</em></td>
<td><img src="docs/screenshots/browser.png" alt="wede built-in browser preview showing wikipedia.org" width="400"><br><em>Built-in browser preview</em></td>
</tr>
</table>

See [docs/SCREENSHOTS.md](docs/SCREENSHOTS.md) for the full gallery and how to regenerate.

---

## Collaboration

wede turns one machine into a shared workspace for your whole team — no accounts, no cloud, no external services.

- **Share links + roles** — the owner mints invite links (`?invite=…`) scoped to a role: **editor** (full access, including terminals) or **viewer** (read-only — no terminal, file writes, or git mutations). Viewers *may* post to the shared **public chat** channel (a deliberately public conversation that persists to `.wede/chat.md`); they cannot write any other file. Tokens are hashed at rest and compared in constant time; the owner can list and revoke them anytime.
- **Workspaces** — open multiple independent projects on one host and switch between them from the top bar. Everyone connected sees the same set of workspaces.
- **Multiplayer presence & cursors** — see who else is in a workspace and which file they're viewing, with live cursors. Collaborative editing is CRDT-backed (pure-Go [reearth/ygo](https://github.com/reearth/ygo), Yjs-compatible).
- **Shared terminals** — everyone in a workspace shares the same PTY sessions: open a terminal and your teammates see the same output in real time.
- **Workspace chat** — per-workspace chat with two channels: **public** (committed to `.wede/chat.md` so the repo — and any LLM working on it — can read the conversation) and **private** (stored in `.wede/private/`, which wede auto-gitignores). Git activity (commits, uncommitted-change counts) is posted into the chat automatically.
- **Public tunnel** — one-click expose a loopback-bound wede to the internet via *your own* sovereign [Vulos Relay](https://github.com/vul-os/vulos-relay) server (owner-only). wede embeds the relay agent — it dials your relay over a single outbound connection and shows the live public URL — no third-party binary, inbound ports, or static IP needed.

> **Security — editor links grant full host shell access.**
> An editor share link gives the recipient a login shell (`$SHELL -l`) running as
> the OS user that started wede, with the complete process environment and no
> filesystem sandbox. The same session also opens the LSP and DAP sockets, which
> spawn language-server and debugger processes as children of the wede process.
> Treat editor links like SSH keys — share them only with people you trust with a
> shell on that machine. Viewer links are safe for read-only access.
> See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#no-sandbox-warning-for-shared-deployments)
> for the full role model and security model.

---

## Features

| Feature | Description |
|---------|-------------|
| **Real-time Collaboration** | Multi-user editing with multiplayer cursors and presence ("who's viewing what"), CRDT-backed via pure-Go reearth/ygo (Yjs-compatible). |
| **Workspaces** | Open multiple independent projects on one host and switch between them; everyone connected shares the same set. |
| **Share Links + Roles** | Owner mints invite links scoped to **editor** (full) or **viewer** (read-only) roles. Tokens hashed at rest, constant-time compare, listable and revocable. |
| **Workspace Chat** | Per-workspace chat with **public** (committed `.wede/chat.md`, LLM-readable) and **private** (auto-gitignored) channels, plus automatic git-activity messages. |
| **Public Tunnel (Vulos Relay)** | One-click expose wede to the internet via your own sovereign Vulos Relay server (owner-only) — embedded relay agent dials out and shows the live URL. No third-party binary, inbound ports, or static IP. |
| **File Explorer** | VS Code-style project tree with git status colours. Context menu: copy, paste (recursive), rename, delete with confirmation. File-watching via SSE auto-refreshes on disk changes. |
| **Code Editor** | CodeMirror 6 with syntax highlighting for JavaScript, TypeScript, Go, Python, Rust, and 10+ languages. Multi-cursor (Alt+Click), column select (Alt+Drag), bracket matching, code folding. |
| **Auto-save** | 1.5 s debounced save after each edit. Status indicator in the top bar. Toggle per-session in Settings. Manual Ctrl/Cmd+S always works. |
| **Project Search** | Ctrl/Cmd+Shift+F — workspace-wide search with ripgrep (Go walker fallback). Case and regex toggles. Replace across files. Results grouped by file; click to jump to exact line. |
| **Command Palette** | Ctrl/Cmd+Shift+P — fuzzy-search over all IDE commands: save, new file/folder, toggle terminal, git ops, theme switch, logout, and more. |
| **Web Terminal** | Full PTY terminal emulator via xterm.js and WebSocket. **Multiple terminals per workspace** (dockable or floating windows), shared live with collaborators. Run shell commands, SSH, Docker — anything. |
| **Tasks** | Named build/test/run commands from `~/.wede/tasks.json`, listed in a Tasks panel and run in a terminal ([docs](docs/GETTING-STARTED.md#tasks--named-buildtestrun-commands)). |
| **Git Client** | VS Code-grade: visual commit graph with branches &amp; merges, blame, side-by-side diff, staging + per-hunk staging, cherry-pick / revert / reset / merge, tags, branch management, push/pull/fetch, stash, merge-conflict resolution. |
| **Comprehensive Search** | Workspace-wide search with regex, case &amp; whole-word toggles, include/exclude globs, context lines, and a filename mode — plus search &amp; replace across files. |
| **Built-in Browser** | Preview your running web app in an embedded browser tab without leaving the IDE. |
| **API Client** | Postman-style HTTP client: methods, params, headers, auth (bearer/basic/api-key), JSON/form/raw bodies, environments with `{{variables}}`, and a server-side send (no CORS). Requests are saved as files under `.wede/requests/` — committable and shareable. |
| **LSP** | Language Server Protocol proxy for diagnostics, hover, completion, and go-to-definition. Ships with gopls, typescript-language-server, pylsp, rust-analyzer — and **any other LSP server can be added without recompiling** via `~/.wede/lsp.json` (see [Adding languages](docs/GETTING-STARTED.md#adding-language-support)). |
| **Syntax highlighting** | 25+ languages out of the box — Go, JS/TS/JSX, Python, Rust, C/C++, Java, PHP, C#, Kotlin, Scala, Swift, Ruby, Lua, shell, PowerShell, SQL, YAML, TOML, JSON, Markdown, HTML/CSS, Dockerfile, INI, and more. |
| **Format on Save** | Auto-formats on Ctrl/Cmd+S: `gofmt` for Go, `prettier` for JS/TS/CSS/JSON/HTML/Markdown, `black` for Python — and **any other formatter** via `~/.wede/formatters.json` ([docs](docs/GETTING-STARTED.md#format-on-save-for-any-language)). |
| **Image & Binary Preview** | Images render inline with a checkerboard background; other binary files show a size notice instead of garbled editor content. |
| **Editor Settings** | Font size, tab width, word wrap, minimap, auto-save — all live-applied without reopening files, persisted to `localStorage`. |
| **Dark & Light Themes** | Midnight (dark) and Daylight (light) colour schemes with Space Grotesk / Inter / JetBrains Mono font stack. |
| **Mobile Friendly** | Fully responsive UI for tablets and phones. |
| **Secure Access** | Owner password with 3-attempt lockout (persisted across restarts). Per-user share tokens with viewer/editor roles, hashed at rest + constant-time compare. Session TTL, server-side logout, WebSocket token via `auth.<token>` subprotocol (recommended, so it stays out of URLs/logs) — a `?token=` query param is also accepted as a fallback, WS origin checks. |
| **Single binary** | Go embeds the entire frontend — one ~10 MB file to deploy anywhere. |

---

## Quick start

Because wede is now community-maintained (see the status note above), the
recommended install is **build from source** — you get a binary you built and can
audit yourself, with no reliance on a hosted release artifact:

```bash
git clone https://github.com/vul-os/wede.git
cd wede
npm install
npm run build:all   # outputs ./wede (single binary with embedded frontend)
```

Requires Go 1.25+ and Node.js 18+. Then run it against a project:

```bash
./wede /path/to/your/project
```

On first run set a strong password in `wede.config.json` (start from
[`wede.config.example.json`](wede.config.example.json)), then open
[http://localhost:9090](http://localhost:9090) and log in.

> **Convenience installer (`install.sh`).** The repo also ships an `install.sh`
> that pulls a prebuilt binary from [GitHub Releases](https://github.com/vul-os/wede/releases)
> via the `releases/latest` API. Since wede is no longer actively maintained, a
> matching release asset for your OS/arch is **not guaranteed to exist**, and the
> script performs **no checksum verification** — so we no longer recommend piping
> it straight into a shell. If you use it, download and read it first, and verify
> the downloaded binary against the checksums on the release page:
>
> ```bash
> curl -fsSLO https://raw.githubusercontent.com/vul-os/wede/main/install.sh
> less install.sh          # review before running
> bash install.sh
> ```

---

## Configuration

wede reads a single `wede.config.json`. It searches, in order: the working
directory and its parents, `~/.config/wede/`, then next to the binary. A
`password` is required; everything else has a safe default.

```json
{
  "password": "your-strong-password-here",
  "port": "9090",
  "host": "127.0.0.1",
  "frame_ancestors": ""
}
```

| Key | Default | Description |
|-----|---------|-------------|
| `password` | *(required)* | Login password. Brute-force lockout after 3 failed attempts, persisted across restarts. |
| `port` | `9090` | HTTP port. Override at runtime with `--port` / `-p`. |
| `host` | `127.0.0.1` | Bind address. Loopback only by default; set to `0.0.0.0` to expose on the network. |
| `frame_ancestors` | `""` | CSP `frame-ancestors` allow-list for iframe embedding. Empty denies all cross-origin framing; set to e.g. `https://vulos.org` to embed in the Vulos OS shell. |

The config file holds your password and is gitignored by default — never commit
it. Start from [`wede.config.example.json`](wede.config.example.json).

### Remote access

wede binds to `127.0.0.1` by default. To reach it from elsewhere you can bind to
the LAN (`"host": "0.0.0.0"`), run it as an app inside **Vulos** (the Vulos
gateway handles routing and auth — no exposed port), or put it on the public
internet via your own **[Vulos Relay](https://github.com/vul-os/vulos-relay)
server** — wede's embedded relay agent dials out from your machine, so no inbound
ports or static IP are needed.
See [Exposing wede over a network](docs/GETTING-STARTED.md#exposing-wede-over-a-network)
for setup.

CLI flags: `wede [path]` opens a workspace directly; `--port`/`-p` overrides the
port; `--version` prints the version. See [docs/CONFIGURATION.md](docs/CONFIGURATION.md)
for the full reference.

---

## Extensibility

wede extends through **configuration, not a binary plugin host** — you add
capabilities by pointing it at tools already on the machine, with no recompile.
Each registry lives in `~/.wede/` (or a trusted project-local `.wede/`) and is
merged over the built-ins at startup:

| Extend | Config | Built-ins |
|--------|--------|-----------|
| **Language servers (LSP)** | `~/.wede/lsp.json` | `gopls`, `typescript-language-server`, `pylsp`, `rust-analyzer` |
| **Debug adapters (DAP)** | `~/.wede/debug.json` | `dlv` (Go), `debugpy` (Python) |
| **Formatters** | `~/.wede/formatters.json` | per-language defaults |
| **Tasks / runners** | `~/.wede/tasks.json` | — |

LSP brings completion, diagnostics, hover and rename; DAP brings breakpoints,
stepping, the call stack and variables — both speak the same standards as VS Code,
so any language server or debug adapter drops in. See
[docs/GETTING-STARTED.md](docs/GETTING-STARTED.md) for copy-paste configs.

> **VS Code `.vsix` extensions are not supported** — they require a Node.js
> extension host, intentionally outside wede's single-binary design; LSP + DAP are
> the portable, editor-agnostic equivalents. A native **WASM plugin API** (sidebar
> panels + editor commands) is planned — see [ROADMAP.md](ROADMAP.md) — with no
> extension *marketplace* by design.

---

## Documentation

| Document | Description |
|----------|-------------|
| [docs/GETTING-STARTED.md](docs/GETTING-STARTED.md) | Installation, first steps, network exposure |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Internal structure, API surface, security model |
| [docs/CONFIGURATION.md](docs/CONFIGURATION.md) | All config keys, iframe embedding, CLI flags |
| [docs/SCREENSHOTS.md](docs/SCREENSHOTS.md) | Screenshot gallery + how to regenerate |
| [ROADMAP.md](ROADMAP.md) | Planned features by milestone |
| [CHANGELOG.md](CHANGELOG.md) | Full version history |

---

## Development

**Prerequisites:** Go 1.25+, Node.js 18+

**Frontend** (React + Vite, hot reload):

```bash
npm install
npm run dev
```

**Backend** (Go):

```bash
cd backend
go run ./cmd/wede .
```

The Vite dev server proxies `/api` and WebSocket requests to the Go backend at port 9090.

**Production build** (single binary with embedded frontend):

```bash
npm run build:all
# outputs ./wede
```

**Tests and lint:**

```bash
cd backend && go test ./...
npm run lint
```

**Regenerate screenshots:**

```bash
npm install                          # installs playwright devDep
npx playwright install chromium      # one-time chromium download
npm run screenshots                  # auto-starts wede on scripts/demo-workspace/
```

The screenshotter starts the `./wede` binary pointed at `scripts/demo-workspace/` automatically. See [docs/SCREENSHOTS.md](docs/SCREENSHOTS.md) for environment variables and route details.

> **Security reminder:** Always set a strong, unique password in `wede.config.json` before exposing wede over a network. The example config uses a placeholder — **change it before use**. The `install.sh` installer auto-generates a random password; if you configured manually, update `wede.config.json` now.

---

## Contributing

Contributions are welcome!

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Commit your changes: `git commit -m 'feat: add my feature'`
4. Push to the branch: `git push origin feat/my-feature`
5. Open a pull request

Please keep the Go tests and lint clean (`go test ./...` + `npm run lint`) before submitting.

---

## License

[MIT](LICENSE) — free to use, modify, and distribute.

### Third-party notices

wede redistributes third-party software: Go modules compiled into the backend
binary, the npm packages bundled into the embedded React IDE (CodeMirror, xterm,
Yjs, React, Tailwind and more — including the **Inter**, **JetBrains Mono** and
**Space Grotesk** webfonts, whose OFL-1.1 licence must travel with the shipped
`.woff2` files), and the mermaid/marked bundles vendored into the marketing
site. Their licences (MIT, BSD, ISC, Apache-2.0, MPL-2.0, OFL-1.1) require the
copyright notice and licence text to accompany every copy.

- [THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt) — name, version, licence and
  full text for every component. Generated from the real dependency graph by
  `make notices` (`scripts/gen-notices.sh`: go-licence-detector for Go,
  license-checker for npm); never hand-edited.
- A running wede serves it at **`/licenses.txt`** (public, before the sign-in
  gate); the marketing site links it from its footer.
- Vendored site bundles carry their upstream licence next to them, e.g.
  `site/assets/vendor/mermaid.min.js.LICENSE`.

---

<div align="center">

<a href="https://github.com/vul-os/wede">GitHub</a> · <a href="https://github.com/vul-os/wede/issues">Issues</a> · <a href="https://github.com/vul-os/wede/releases">Releases</a>

<br>

<sub>wede is a free, open-source, self-hosted <strong>collaborative</strong> web IDE and remote development environment.<br>
Built as an alternative to code-server, VS Code Server, Gitpod, and GitHub Codespaces.<br>
Keywords: collaborative web IDE, self-hosted IDE, real-time pair programming, multiplayer code editor,<br>
shared terminal, browser code editor, remote development, online terminal, git client,<br>
open source IDE, developer tools, Go web server, single binary IDE.</sub>

</div>

---

<sub><img src="docs/assets/vulos-logo.png" height="16" alt="VulOS"> · <strong>Built with purpose. Open by design.</strong></sub>
