# wede Screenshots

Visual tour of the IDE. All screenshots are captured at 1440×900.

---

## Gallery

### Login

![Login](screenshots/login.png)

The password authentication screen.

---

### IDE — Editor + File Tree

![IDE full view](screenshots/hero.png)

Full IDE layout: file explorer on the left, code editor in the centre, top bar with workspace controls.

---

### Git Panel

![Git panel](screenshots/git.png)

Staging area with inline diff view.

---

### Git commit graph

![Git commit graph](screenshots/git_graph.png)

Visual SVG DAG of branch/merge history. Right-click commits for context menu.

---

### Search Panel

![Search panel](screenshots/search.png)

Workspace-wide search with ripgrep. Supports regex, case-toggle, and replace-across-files.

---

### Terminal

![Terminal](screenshots/terminal.png)

Full PTY terminal panel with multiple tabs. Run shell commands, SSH, Docker — anything.

---

### Settings

![Settings](screenshots/settings.png)

Editor settings: font size, tab width, word wrap, auto-save, minimap, LSP, and theme picker.

---

### Command Palette

![Command palette](screenshots/command_palette.png)

`Ctrl+Shift+P` — fuzzy-search over all IDE commands.

---

### Mobile layout

![Mobile view](screenshots/mobile.png)

Fully responsive layout for tablets and phones.

---

### Built-in browser preview

![Browser preview](screenshots/preview.png)

Embedded browser tab for previewing your running app.

---

### Light theme (Daylight)

![Light theme](screenshots/full_light.png)

Daylight colour scheme.

---

## Regenerating screenshots

See the [Development section in README.md](../README.md#development) or run:

```bash
npm run screenshots
```

### Prerequisites

- Node.js 18+
- `npm install` (installs Playwright)
- `npx playwright install chromium`
- wede running locally (default: `http://localhost:9090`)
- A workspace open in wede (pass a path on startup: `wede /path/to/project`)

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BASE_URL` | `http://localhost:9090` | wede instance URL |
| `WEDE_PASSWORD` | `admin` | Login password |

### Routes captured

| Screenshot file | Route / action |
|----------------|----------------|
| `login.png` | `/` before login |
| `hero.png` | IDE main view (editor + file tree) |
| `git.png` | Git panel — Changes tab with diff |
| `git_graph.png` | Git panel — History (commit graph) |
| `search.png` | Search panel (`Ctrl+Shift+F`) |
| `terminal.png` | Terminal panel |
| `settings.png` | Settings panel |
| `command_palette.png` | Command palette (`Ctrl+Shift+P`) |

### Notes

- Screenshots that require a workspace with git history (git graph, diff) are best-effort — the script opens the IDE pointed at the wede repo itself.
- If wede cannot start in the CI environment, the script writes `docs/screenshots/README.md` with a note and exits cleanly.
