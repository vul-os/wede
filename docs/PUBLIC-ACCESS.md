# Exposing wede publicly

wede binds to `127.0.0.1` by default and is a single static binary — it has no
opinion on how you reach it from the public internet. There are three ways to
do it. **The built-in relay tunnel is one option among these three, not the
only way** — pick whichever fits your setup.

| Option | Open inbound ports? | Needs a VPS/DNS you control? | Setup effort |
|---|---|---|---|
| (a) Direct bind + reverse proxy | Yes (443 on your box) | No (just a domain) | Low |
| (b) Generic outbound tunnel | No | Depends on provider | Low–Medium |
| (c) Built-in Vulos Relay tunnel | No | Yes (your own relay VPS) | Low, sovereign |

---

## (a) Direct bind + reverse proxy

If your machine has a public IP (or a router with port forwarding) and you're
comfortable managing TLS yourself, put a reverse proxy in front of wede and
bind wede itself to loopback — the proxy is the only thing exposed.

`wede.config.json`:

```json
{
  "password": "your-strong-password-here",
  "port": "9090",
  "host": "127.0.0.1"
}
```

Keep `host` at `127.0.0.1` even though wede is reachable publicly — the proxy
runs on the same machine and talks to it over loopback; wede itself never
binds a public interface.

### Caddy

Caddy gets you automatic HTTPS (Let's Encrypt) with no extra steps:

```
wede.example.com {
    reverse_proxy 127.0.0.1:9090
}
```

That's the whole `Caddyfile`. Run `caddy run`, point `wede.example.com`'s DNS
A/AAAA record at the box, and Caddy handles the certificate and TLS
termination.

### nginx

```nginx
server {
    listen 443 ssl;
    server_name wede.example.com;
    ssl_certificate     /etc/letsencrypt/live/wede.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/wede.example.com/privkey.pem;

    location / { proxy_pass http://127.0.0.1:9090; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade"; proxy_set_header Host $host; }
}
```

The `Upgrade`/`Connection` headers matter: wede's terminal, LSP, and collab
features are WebSocket-based, so the proxy must forward upgrade requests.

If you'd rather not run wede on `0.0.0.0` at all (see
[CONFIGURATION.md](CONFIGURATION.md)) — the reverse proxy above already keeps
wede loopback-only, which is the safer default. Only set `host: "0.0.0.0"` if
your proxy runs on a *different* machine than wede.

---

## (b) A generic outbound tunnel

If you don't have a public IP or don't want to manage a proxy/TLS yourself,
any outbound-dialing tunnel product gets you the same "no open inbound ports"
property as wede's built-in relay — point it at wede's local port
(`127.0.0.1:9090` by default) and it publishes a URL:

- **Cloudflare Tunnel** (`cloudflared tunnel --url http://127.0.0.1:9090`) — free, Cloudflare-fronted, good if you're already on their DNS.
- **ngrok** (`ngrok http 9090`) — fastest to try, ephemeral URLs on the free tier.
- **frp** — the original self-hosted reverse-tunnel project; run `frps` on a VPS and `frpc` next to wede, same shape as wede's own relay but third-party.
- **Tailscale Funnel** (`tailscale funnel 9090`) — no separate VPS at all if you're already on a tailnet; publishes through Tailscale's infrastructure.

All four dial *out* from the wede machine, same as the built-in relay — none
of them need you to open a port or have a static IP.

---

## (c) The built-in Vulos Relay tunnel

wede embeds a first-party tunnel agent
(`github.com/vul-os/vulos-relay/tunnel/agent`) that does the same job as
options (b) above, sovereign: you run your own relay server instead of
trusting a third party's. It's the **default** `Provider` behind wede's
`Tunnel` interface (`backend/internal/tunnel`) — not a hard-wired requirement.
An alternate `Provider` implementation (wrapping any of the tools in (b), for
example) could be swapped in via `tunnel.NewWithProvider` without touching
`main.go` or the HTTP handlers.

See [GETTING-STARTED.md § Public internet — a sovereign Vulos Relay](GETTING-STARTED.md#3-public-internet--a-sovereign-vulos-relay)
for the walkthrough (owner-only **Settings → Public access** panel, config
persisted to `~/.wede/tunnel.json`).

---

## Which one should I use?

- Already run Vulos OS? Use [embedding](GETTING-STARTED.md#2-through-vulos--no-public-exposure) instead — no public exposure needed at all.
- Have a domain + a box with a public IP? (a) is the least moving parts.
- No public IP, don't want to run anything extra? (b), pick whichever tunnel product you already trust.
- Want to self-host the tunnel infrastructure too, not just wede? (c).
