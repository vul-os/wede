<!-- Generated from docs/CONFIGURATION.md by scripts/sync-docs.mjs — edit the source, not this file. -->

# wede Configuration

wede is configured via a single JSON file: `wede.config.json`. This document
covers all six config keys, how the file is found and parsed, CLI flags, and
override precedence.

---

## Config file location

wede searches for `wede.config.json` in this order, using the **first file
found**:

1. The current working directory, then each parent directory up to `/`
2. `~/.config/wede/wede.config.json`
3. The directory containing the `wede` binary

If no file is found in any of these locations, wede logs an error naming all
four searched locations and exits (`log.Fatal`) — there is no built-in
zero-config default. This means you can place the config in your project root
and run `wede` from any subdirectory of it, and it will still be found.

---

## Complete key reference

| Key | Type | Default | Env override | Description |
|-----|------|---------|---------------|-------------|
| `password` | `string` | *(none — required)* | — | Login password. Startup fails if missing or empty. |
| `port` | `string` | `"9090"` | — | TCP port to listen on. Overridable at the CLI with `--port`/`-p`. |
| `host` | `string` | `"127.0.0.1"` | — | Interface to bind to. |
| `frame_ancestors` | `string` | `""` | — | Space-separated origins allowed to embed wede in an `<iframe>`. |
| `workspace_root` | `string` | `$HOME` | `WEDE_WORKSPACE_ROOT` | Base directory that confines which paths can be opened as a workspace. |
| `serve_landing` | `bool` | `false` | — | Whether the marketing landing page and `/site/*` docs viewer are mounted at all. |

Only `port` (CLI flag) and `workspace_root` (environment variable) have an
override mechanism beyond the config file itself; the other four keys are
config-file-only.

---

## `password`

Required. There is no default — `wede.config.json` without a `password` key
(or with `"password": ""`) fails to start with `password is required in
wede.config.json`. Used for the single owner login; 3 failed attempts trigger
a lockout that persists across restarts (see [ARCHITECTURE.md](#architecture)
for the auth model). Choose a strong, unique value — see
[Security: change the default password](#security-change-the-default-password)
below.

## `port`

Default `"9090"`. TCP port the HTTP server listens on. A JSON string, not a
number (e.g. `"8080"`, not `8080`). Overridable at runtime with the CLI flags
`--port <port>` or `-p <port>` (see [CLI flags](#cli-flags)) — the flag wins
over whatever is in the config file.

## `host`

Default `"127.0.0.1"` — binds to loopback only, reachable solely from the
machine wede runs on. Set to `"0.0.0.0"` to listen on all interfaces, which is
required when wede is accessed from another machine, a container, or a Docker
host.

> **Note on empty string:** although the field comment in the source
> describes `""` as equivalent to `"0.0.0.0"`, the running server does not
> treat it that way: `main.go` falls back to `"127.0.0.1"` whenever the
> resolved `host` value is empty (whether that's because the key was omitted
> or explicitly set to `"host": ""`). In practice, use `"0.0.0.0"` explicitly
> to bind to all interfaces — an empty string binds to loopback, same as the
> default.

## `frame_ancestors`

Default `""`. Controls which origins may embed wede in an iframe, emitted as
`Content-Security-Policy: frame-ancestors <value>`.

- **Empty (default):** wede sends `X-Frame-Options: DENY` and
  `Content-Security-Policy: frame-ancestors 'self'` — all cross-origin
  framing is blocked.
- **Non-empty:** a space-separated list of origins, e.g.
  `"https://vulos.org https://app.vulos.org"`. `X-Frame-Options` is omitted
  (it can't express multiple origins) so the CSP directive takes sole effect,
  and WebSocket origin checks are widened to allow connections from the
  listed origins.

The standalone experience (direct browser access, not embedded) is unaffected
either way. See [Embedding in an iframe](#embedding-in-an-iframe) below.

## `workspace_root`

Default (when omitted): the user's home directory (`$HOME`), resolved to an
absolute path. Can also be set via the `WEDE_WORKSPACE_ROOT` environment
variable, which takes priority over the config-file value (see
[Precedence](#precedence)).

This is a **security control**, not just a convenience default: it confines
which directories can be adopted as a workspace at runtime. Any path passed
to `POST /api/folder/open` or `POST /api/workspaces` must, after `~`
expansion and symlink resolution:

- resolve to a location **inside** `workspace_root` (or be `workspace_root`
  itself),
- not be the filesystem root (`/`),
- not be the user's **home directory itself** — rejected outright regardless
  of what `workspace_root` is set to,
- not contain a dotfile path component below the allowed base (e.g. `.ssh`,
  `.config`, `.gnupg`) or a `..` traversal segment.

A request that fails any of these checks gets `400 Bad Request` with a JSON
`{"error": "..."}` body describing which rule was violated (e.g. `"path
outside the allowed workspace root (...)"`, `"dotfile directories are not
allowed in a workspace path (.ssh)"`, `"refusing to open the home directory
itself as a workspace"`). This stops an authenticated editor from adopting a
sensitive directory (`$HOME`, `/`, `~/.ssh`) as a workspace root and reading
its contents through the IDE.

**Exception — the boot path.** The directory you pass on the command line at
launch (`wede /path/to/project`) is the owner's explicit, one-time launch
choice and is **exempt** from this restriction: it only needs to exist and be
a directory. `workspace_root` governs paths opened *after* startup, through
the UI or API, not the initial boot workspace.

**Separately:** the folder-picker's browse endpoint (`GET
/api/folder/browse`, used to list subdirectories in the UI) is always
confined to the user's home-directory tree, independent of `workspace_root`.
Setting `workspace_root` to a directory outside `$HOME` does not widen what
the picker can browse.

If you never set `workspace_root`, the default (`$HOME`) is a broad but safe
base: it blocks `/`, `$HOME` itself, and dotfiles, but still lets an editor
open any non-dotfile subdirectory of your home folder. Set it explicitly to a
narrower directory (e.g. `/srv/projects`) to restrict wede to only that tree.

## `serve_landing`

Default `false`. Gates whether the marketing landing page and the `/site/*`
docs viewer are mounted at all. These are assets built for the public cloud
deployment (`wede.vulos.org`) — a local, self-hosted instance normally has no
use for them.

When `false` (the default for every self-hosted install):
- `/site/*` is **not mounted** — visiting `/site/` returns a plain 404. This
  is expected behavior for a self-hosted instance, **not a bug**.
- Unauthenticated visits to `/` are served the IDE's own login form (the SPA)
  instead of a marketing landing page.

When `true`:
- `/site/*` serves the bundled landing page + docs viewer, and `/site`
  redirects to `/site/`.
- Unauthenticated visits to `/` are served the landing page instead of the
  bare login form (authenticated visits always get the IDE regardless).

Set this to `true` only if you are deliberately hosting the public marketing
site alongside the IDE (as the Vulos cloud deployment does). A normal
self-host does not need it.

> **Note:** `GET /licenses.txt` (third-party notices) is **not** gated by
> `serve_landing` — it is mounted unconditionally whenever the binary was
> built with notices embedded, on every deployment, self-hosted or not.

---

## Strict parsing: unknown keys are rejected

wede decodes `wede.config.json` with `json.Decoder.DisallowUnknownFields()`.
Any key not in the six above causes an immediate startup error of the form
`invalid wede.config.json: json: unknown field "..."`, rather than silently
ignoring it. A typo like `"frame_ancestor"` (missing `s`) is caught at
launch, not swallowed.

---

## Security: change the default password

> **Always set a strong, unique password before exposing wede over a
> network.**

The `install.sh` installer auto-generates a random 16-character password. If
you created the config manually, choose a strong password yourself:

```json
{
  "password": "correct-horse-battery-staple-42",
  "port": "9090"
}
```

Do not commit `wede.config.json` — it is gitignored by default.

---

## Embedding in an iframe

By default wede sets `X-Frame-Options: DENY` and `Content-Security-Policy:
frame-ancestors 'self'`, blocking all cross-origin embedding.

To allow the Vulos OS app shell (or any other trusted origin) to embed wede
in an iframe, set `frame_ancestors`:

```json
{
  "password": "your-password",
  "port": "9090",
  "frame_ancestors": "https://vulos.org https://app.vulos.org"
}
```

When `frame_ancestors` is non-empty:
- `Content-Security-Policy: frame-ancestors <value>` is emitted (modern
  browsers honour this)
- `X-Frame-Options` is omitted (it cannot express multiple origins)
- WebSocket origin checks allow connections from the listed origins

The standalone experience (direct browser access) is unaffected.

---

## Exposing over a network

By default wede binds to `127.0.0.1` — only accessible from the local
machine. To access from another machine or through a reverse proxy:

```json
{
  "password": "your-strong-password-here",
  "port": "9090",
  "host": "0.0.0.0"
}
```

> **Warning:** Binding to `0.0.0.0` exposes wede on all network interfaces.
> Always use a strong password and consider TLS via a reverse proxy (nginx,
> Caddy, etc.) when accessible over the internet. See
> [GETTING-STARTED.md](#getting-started:exposing-wede-over-a-network) and
> [PUBLIC-ACCESS.md](#public-access) for full options.

---

## CLI flags

```
wede [flags] [path]
```

| Flag | Description |
|------|-------------|
| `--port <port>` | Override the port from config |
| `-p <port>` | Shorthand for `--port` |
| `--version` | Print version and exit |
| `path` | Directory to open as the initial workspace on startup (optional; exempt from the `workspace_root` restriction — see [`workspace_root`](#workspace_root)) |

Example:

```bash
wede --port 8080 /home/user/myproject
```

---

## Precedence

From highest to lowest priority:

1. **CLI flags** — `--port`/`-p` overrides `port` from the config file, applied
   right after the config is loaded. The positional `path` argument sets the
   initial workspace directly and isn't a config value at all.
2. **Environment variable** — `WEDE_WORKSPACE_ROOT` overrides `workspace_root`
   from the config file.
3. **Config file** (`wede.config.json`) — the values you set explicitly.
4. **Built-in defaults** — `port: "9090"`, `host: "127.0.0.1"`,
   `frame_ancestors: ""`, `workspace_root: $HOME`, `serve_landing: false`;
   `password` has no default and is required.

Only `port` and `workspace_root` have an override layer above the config
file — `password`, `host`, `frame_ancestors`, and `serve_landing` can only be
set in `wede.config.json`.

---

## Example configs

All three examples below pass strict parsing: each uses only real key names,
and every field except `password` is optional (defaults apply for anything
omitted).

### Minimal (local use)

```json
{
  "password": "my-secret-password"
}
```

Uses every default: port `9090`, loopback-only `host`, no iframe embedding,
`workspace_root` = `$HOME`, and `serve_landing` off.

### NAS / home server

```json
{
  "password": "my-secret-password",
  "port": "9090",
  "host": "0.0.0.0"
}
```

### Embedded in Vulos OS

```json
{
  "password": "my-secret-password",
  "port": "9090",
  "frame_ancestors": "https://vulos.org"
}
```

### Restricting the workspace root

```json
{
  "password": "my-secret-password",
  "workspace_root": "/srv/projects"
}
```

Confines every workspace opened after startup to `/srv/projects` (instead of
the `$HOME`-wide default), rejecting anything outside that tree, dotfile
paths within it, and traversal attempts.
