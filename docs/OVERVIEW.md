# wede

**A self-hosted, collaborative web IDE in a single Go binary.**

wede serves a full IDE — editor, terminals, git, search, chat — straight off a machine
you control. Point it at a folder, open a browser, and you have a workspace. Send someone
a link and they are in the same workspace with you: same files, same cursors, same
terminal, same chat thread.

There is no cloud service behind it, no container to orchestrate, no database to run, and
no account to create. One process, one file on disk, your hardware.

> **Status: deprioritized.**
> wede is **not under active development** and is effectively community-maintained. It has
> not been deprecated or removed, and nothing has stopped working — but expect no roadmap
> work, and use it at your own risk. If you depend on it, [build from
> source](GETTING-STARTED.md), pin a commit, and be prepared to maintain your own fork.

---

## What you get

| | |
|---|---|
| **Real-time collaboration** | Concurrent editing with multiplayer cursors and a presence roster showing who is in the workspace and which file they have open. CRDT-backed by pure-Go [reearth/ygo](https://github.com/reearth/ygo), wire-compatible with Yjs. |
| **Shared terminals** | Real PTYs streamed over WebSocket to every collaborator at once — same session, same output. Dockable or floating, multiple per workspace, renameable. |
| **VS Code-grade git** | Visual commit graph, blame, side-by-side diffs, per-hunk staging, conflict resolution, cherry-pick, revert, reset, merge, stash, tags, branches, remotes. Drives your system `git` binary. |
| **Workspaces & share links** | Several projects open on one host, multi-root, switchable. Invite links scoped **editor** or **viewer**, with optional TTL, revocable at any time. |
| **Workspace chat** | A public channel that *is* `.wede/chat.md` — committed with your code and readable by your tooling — plus a private, auto-gitignored channel. Git activity posts itself into the thread. |
| **Language tooling** | LSP for diagnostics, hover, completion and go-to-definition (gopls, typescript-language-server, pylsp, rust-analyzer wired in); DAP debugging (`dlv`, `debugpy`); format-on-save (`gofmt`, `prettier`, `black`). All extensible by config, no recompile. |
| **Project search** | ripgrep when `rg` is on PATH, a pure-Go walker when it isn't. Regex, whole-word, include/exclude globs, context lines, and search-and-replace behind a preview. |
| **API client** | Postman-shaped requests sent server-side, so CORS never applies. `{{variables}}`, environments, collections saved under `.wede/requests/`. Blocks loopback, private-range and cloud-metadata targets by default. |
| **Editor** | CodeMirror 6, 27 languages, minimap, multi-cursor, column select, folding, image preview, 1.5 s debounced autosave, command palette. |
| **Themes & mobile** | Midnight (dark) and Daylight (light), self-hosted Space Grotesk / Inter / JetBrains Mono — no runtime font CDN. Fully responsive down to a phone. |

Full feature detail lives in the [README](https://github.com/vul-os/wede#features).

---

## Deployment modes

wede runs three ways, all on hardware you control.

**Standalone.** The default. The binary listens on `127.0.0.1:9090`, you log in with the
owner password, and collaborators reach it over your LAN or through a reverse proxy. See
[Getting started](GETTING-STARTED.md).

**Behind a proxy, on the public internet.** Terminate TLS at Caddy or nginx, or use the
built-in outbound tunnel and never open an inbound port. Both paths, with copy-pasteable
configs, are in [Public access](PUBLIC-ACCESS.md).

**Embedded in Vulos OS.** Set `frame_ancestors` to the host shell and wede loads as a
first-class app inside [Vulos](https://vulos.org). See [Configuration](CONFIGURATION.md).

---

## What it costs you to run

| | |
|---|---|
| Binary | ~19 MB, stripped release build, frontend embedded |
| Runtime dependencies | none — no database, no container runtime, no Node at runtime |
| Go dependencies | 5 direct |
| Release targets | linux, macOS, Windows · amd64 and arm64 |
| Default listener | `127.0.0.1:9090`, plain HTTP (TLS is yours to terminate) |
| Build requirements | Go 1.25+, Node.js 18+ |
| Licence | MIT **or** Apache-2.0, your choice |

Optional external tools it will use if it finds them on PATH: `git` (required for the git
features), `rg`, and whichever language servers, debug adapters and formatters you want.
None are bundled.

---

## Before you expose it

One thing to internalise before sending anyone a link:

> **An editor share link grants an unsandboxed login shell** as the OS user running wede,
> with that user's full environment, filesystem access and SSH keys. There is no sandbox.
> Treat editor links like SSH keys. Viewer links are read-only — no terminal, no writes, no
> git mutations — though a viewer can still post in the public chat.

[Hardening](SECURITY-HARDENING.md) walks the whole checklist; [Architecture](ARCHITECTURE.md)
explains where the trust boundaries actually sit.

---

## Where to go next

- **[Getting started](GETTING-STARTED.md)** — build it, configure it, open your first workspace.
- **[Configuration](CONFIGURATION.md)** — every config key, env override and CLI flag.
- **[Deployment](DEPLOYMENT.md)** — run it as a service, back it up, upgrade it.
- **[Public access](PUBLIC-ACCESS.md)** — reverse proxies, TLS and tunnels.
- **[Hardening](SECURITY-HARDENING.md)** — the security checklist and threat model.
- **[Troubleshooting](TROUBLESHOOTING.md)** — symptoms, causes, fixes.
- **[Architecture](ARCHITECTURE.md)** — how the thing is put together.
- **[API reference](API.md)** — the HTTP and WebSocket surface.
- **[Screenshots](SCREENSHOTS.md)** — every screen, and how the shots are regenerated.

---

*wede is part of [Vulos](https://vulos.org) — rooted in **vula**, the Zulu and Xhosa word
for **open**.*
