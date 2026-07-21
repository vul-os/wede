# Security Policy

wede is a self-hosted collaborative web IDE in a single Go binary: real-time
multi-user editing, shared terminals, and git. Because it runs code and shells
on the host, its security boundary matters. Reports are taken seriously and
handled with priority.

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

- Preferred: [GitHub private vulnerability reporting](https://github.com/vul-os/wede/security/advisories/new) on `vul-os/wede`.
- Alternatively, email **vulosorg@gmail.com** with `[wede security]` in the subject.

Include what you can: affected area (auth/session, the collaboration channel, a
shared terminal, git handling), reproduction steps, and impact as you understand
it. You'll get an acknowledgement within **72 hours** and a status update at
least every **14 days** until resolution. Please give a reasonable window to ship
a fix before public disclosure — we'll credit you in the release notes unless
you'd rather stay anonymous.

## Scope

Especially interested in:

- **Authentication & session** — joining a workspace or collaboration session
  without authorization, or hijacking another user's session.
- **Shared terminals & code execution** — any path that lets an unauthorized
  user reach a shell, or escapes the intended workspace boundary on the host.
- **Collaboration channel** — forging edits as another user, or a malicious peer
  corrupting shared state.
- **Git & credential handling** — mishandling of stored credentials or tokens,
  or command injection via repository operations.

Out of scope: vulnerabilities requiring an already-compromised host, and issues
in third-party services the operator configures.

## Supported versions

Only the latest release (and `main`) receives fixes.
