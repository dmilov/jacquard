# Jacquard

> Just as the Jacquard loom wove complex patterns by reading punched cards — turning instructions into fabric — Jacquard weaves your AI CLI sessions into a single observable, controllable surface.

Named after the [Jacquard loom](https://en.wikipedia.org/wiki/Jacquard_machine) — the first programmable machine (1804), ancestor of all computing.

---

## What it does

Jacquard lets you run Claude or Copilot CLI sessions and observe, control, and replay them from a browser — whether the session was started locally or launched remotely from the UI itself.

- **Full PTY passthrough** — the CLI behaves exactly as if run directly in your terminal
- **Live browser terminal** — watch and type in a real xterm.js terminal, not a log viewer
- **Conversation history** — every exchange is recorded to SQLite and searchable later
- **Multi-session** — run as many CLI sessions as you want; each gets its own tile in the sidebar
- **Launch & kill from the browser** — no need to touch the terminal once Switchboard is running

---

## Components

### Loom
*One per CLI process.* Wraps Claude or Copilot in a PTY with full terminal passthrough. Intercepts I/O, records conversations to SQLite, broadcasts live output over WebSocket, and registers with Switchboard. Runs interactively (local terminal) or headlessly (launched by Switchboard).

```
loom [-name "my session"] [-db jacquard.db] -- claude
loom [-name "copilot"] [-db jacquard.db] -- copilot chat
```

### Switchboard
*One per machine.* Registry of all active Loom instances. Serves the Periscope web UI, proxies WebSocket connections to Looms, exposes the HTTP API, and auto-launches Loom subprocesses on demand. Shares the same SQLite file as Loom.

```
switchboard [-addr :1804] [-db jacquard.db]
```

### Periscope
*Web client, served by Switchboard at `/`.* Launch sessions, watch live PTY output in a full xterm.js terminal, type interactively, inject messages, rename sessions, kill sessions, and browse conversation history.

---

## Architecture

```
Browser (Periscope)
       │  HTTP / WebSocket
       ▼
  Switchboard ──── SQLite (conversations + messages)
       │  WebSocket proxy + process spawn
       ▼
    Loom(s)
       │  PTY (ConPTY on Windows, native on Unix)
       ▼
  claude / copilot CLI
```

The WebSocket between browser and Loom is fully bidirectional:
- **Loom → Browser**: raw PTY bytes, rendered by xterm.js
- **Browser → Loom**: keyboard input (`{type:"input"}`) and terminal resize (`{type:"resize"}`)

---

## Quick Start

### Prerequisites
- Go 1.22+

### Build
```bash
go build -o bin/loom        ./cmd/loom
go build -o bin/switchboard ./cmd/switchboard
```

On Windows: `go build -o bin/loom.exe ./cmd/loom` etc.

### Run

**1. Start Switchboard** (once per machine):
```bash
./bin/switchboard -db jacquard.db
```

**2. Either** launch a CLI from your terminal through Loom:
```bash
./bin/loom -name "research" -db jacquard.db -- claude
```

**Or** launch directly from the browser at `http://localhost:1804` — type a command in the sidebar, press Enter.

**3. Open Periscope** at `http://localhost:1804`.

Click a session in the sidebar → the live terminal opens. Click inside the terminal and start typing — keystrokes go straight to the CLI.

---

## Loom flags

| Flag | Default | Description |
|------|---------|-------------|
| `-name` | command string | Display name shown in Periscope |
| `-switchboard` | `http://localhost:1804` | Switchboard URL to register with |
| `-db` | `jacquard.db` | SQLite database path |
| `-id` | _(generated)_ | Loom ID — set by Switchboard when launching headlessly |

---

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/looms` | List active Loom instances |
| `POST` | `/api/looms/launch` | Launch a new headless Loom `{"command":"claude","name":"..."}` |
| `POST` | `/api/looms/{id}/inject` | Send a complete message to a Loom |
| `POST` | `/api/looms/{id}/kill` | Kill a Loom process and deregister it |
| `PATCH` | `/api/looms/{id}` | Rename a Loom `{"name":"new name"}` |
| `GET` | `/api/looms/{id}/ws` | WebSocket: bidirectional terminal stream |
| `GET` | `/api/conversations` | List all conversations (`?node_id=` to filter) |
| `GET` | `/api/conversations/{id}/messages` | Get messages for a conversation |

---

## Database

Both binaries share the same SQLite file via `-db` (default: `jacquard.db` in the working directory). Schema is applied automatically on first run — no setup needed. WAL mode is enabled so Switchboard can read while Loom writes.

---

## How conversation recording works

Loom records every exchange to SQLite in real time:

- **User messages**: assembled from raw keystrokes — backspace, arrow keys, and escape sequences are handled so only the final typed text is saved
- **Assistant messages**: buffered PTY output between two user Enter presses, stripped of ANSI sequences
- Messages injected from the web UI are also recorded
- History is queryable via the Periscope **History** tab or the API
