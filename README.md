# Jacquard

> Orchestrate and observe Claude and Copilot CLI agents across machines.

Named after the [Jacquard loom](https://en.wikipedia.org/wiki/Jacquard_machine) — the first programmable machine (1804), ancestor of all computing.

---

## Components

### Loom
*Per CLI process.* Wraps a Claude or Copilot CLI process in a PTY, providing full transparent terminal passthrough while intercepting I/O. Records the conversation to MySQL and exposes live output over WebSocket. Registers itself with the local Switchboard on startup.

```
loom -switchboard http://localhost:8080 -db jacquard.db -- claude
```

### Switchboard
*Per machine (node).* Maintains a registry of all active Loom instances on the node. Exposes an HTTP API for remote clients to list looms, inject messages, stream live output, and query conversation history. Also serves the Periscope web UI.

```
switchboard -addr :8080 -db jacquard.db
```

### Periscope
*Web client.* Served by Switchboard at `/`. Browse active Loom instances, view live PTY output via WebSocket, inject messages, and search conversation history.

Open `http://localhost:8080` after starting Switchboard.

---

## Architecture

```
Browser (Periscope)
       │  HTTP / WebSocket
       ▼
  Switchboard  ──── SQLite (conversations, messages)
       │  HTTP / WebSocket proxy
       ▼
    Loom(s)
       │  PTY
       ▼
  claude / copilot CLI
       │
       ▼
  Your terminal (transparent passthrough)
```

---

## Quick Start

### Prerequisites
- Go 1.22+

### Build
```bash
go build -o bin/loom        ./cmd/loom
go build -o bin/switchboard ./cmd/switchboard
```

### Run

**1. Start Switchboard** (once per machine):
```bash
./bin/switchboard -db jacquard.db
```
The database file is created automatically on first run. No setup needed.

**2. Launch a CLI session through Loom** (instead of calling it directly):
```bash
./bin/loom -db jacquard.db -- claude
```
The terminal behaves exactly as if you ran `claude` directly. Loom is transparent.

**3. Open Periscope** at `http://localhost:8080` to observe and interact remotely.

---

## API Reference

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/looms` | List active Loom instances |
| `POST` | `/api/looms/{id}/inject` | Inject a message into a Loom |
| `GET` | `/api/looms/{id}/ws` | WebSocket: live PTY output |
| `GET` | `/api/conversations` | List all conversations (`?node_id=` to filter) |
| `GET` | `/api/conversations/{id}/messages` | Get messages for a conversation |

---

## Database

Both binaries share the same SQLite file via the `-db` flag (default: `jacquard.db` in the current directory). The schema is applied automatically on startup — no manual setup required.
