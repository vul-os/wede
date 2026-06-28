# wede Configuration

wede is configured via a single JSON file: `wede.config.json`.

---

## Config file location

wede searches for `wede.config.json` in this order:

1. The current working directory, then each parent directory up to `/`
2. `~/.config/wede/wede.config.json`
3. The directory containing the `wede` binary

The first file found is used. This means you can place the config in your project root and run `wede` from any subdirectory.

---

## Config keys

```json
{
  "password": "your-strong-password-here",
  "port": "9090",
  "host": "127.0.0.1",
  "frame_ancestors": ""
}
```

| Key | Type | Required | Default | Description |
|-----|------|----------|---------|-------------|
| `password` | `string` | **Yes** | — | Password for browser login. Use a strong, unique value. |
| `port` | `string` | No | `"9090"` | TCP port to listen on. |
| `host` | `string` | No | `"127.0.0.1"` | Interface to bind to. `"127.0.0.1"` = localhost only; `"0.0.0.0"` = all interfaces. |
| `frame_ancestors` | `string` | No | `""` | Space-separated origins allowed to embed wede in an `<iframe>`. Empty = block all cross-origin framing. |

> **Strict parsing:** Unknown keys in `wede.config.json` cause an immediate startup error. A typo like `"frame_ancestor"` (missing `s`) will be caught at launch, not silently ignored.

---

## Security: change the default password

> **The default config uses a placeholder password. Always set a strong, unique password before exposing wede over a network.**

The `install.sh` installer auto-generates a random password. If you created the config manually, choose a strong password:

```json
{
  "password": "correct-horse-battery-staple-42",
  "port": "9090"
}
```

Do not commit `wede.config.json` — it is gitignored by default.

---

## Embedding in an iframe

By default wede sets `X-Frame-Options: DENY` and `Content-Security-Policy: frame-ancestors 'self'`, blocking all cross-origin embedding.

To allow the Vulos OS app shell (or any other trusted origin) to embed wede in an iframe, set `frame_ancestors`:

```json
{
  "password": "your-password",
  "port": "9090",
  "frame_ancestors": "https://vulos.org https://app.vulos.org"
}
```

When `frame_ancestors` is non-empty:
- `Content-Security-Policy: frame-ancestors <value>` is emitted (modern browsers honour this)
- `X-Frame-Options` is omitted (it cannot express multiple origins)
- WebSocket origin checks allow connections from the listed origins

The standalone experience (direct browser access) is unaffected.

---

## Exposing over a network

By default wede binds to `127.0.0.1` — only accessible from the local machine. To access from another machine or through a reverse proxy:

```json
{
  "password": "your-strong-password-here",
  "port": "9090",
  "host": "0.0.0.0"
}
```

> **Warning:** Binding to `0.0.0.0` exposes wede on all network interfaces. Always use a strong password and consider TLS via a reverse proxy (nginx, Caddy, etc.) when accessible over the internet.

---

## CLI flags

Config file values can be overridden at the command line:

```
wede [flags] [path]
```

| Flag | Description |
|------|-------------|
| `--port <port>` | Override the port from config |
| `-p <port>` | Shorthand for `--port` |
| `--version` | Print version and exit |
| `path` | Project directory to open on startup |

Example:

```bash
wede --port 8080 /home/user/myproject
```

---

## Example configs

### Minimal (local use)

```json
{
  "password": "my-secret-password"
}
```

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
