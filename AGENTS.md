# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Build

```bash
go build -o bin/loom.exe        ./cmd/loom
go build -o bin/switchboard.exe ./cmd/switchboard
```

Run both from the same directory so they share `jacquard.db` (the default SQLite path).

## Architecture

Three named components, two binaries:

```
cmd/loom/         → loom.exe       (one per CLI session)
cmd/switchboard/  → switchboard.exe (one per machine, serves web UI)
web/periscope/    → embedded in switchboard.exe via //go:embed
```

### Internal packages

| Package | Role |
|---------|------|
| `internal/loom` | PTY wrapper, broadcaster, recorder, agent HTTP server |
| `internal/switchboard` | registry, DB helpers, HTTP server, launcher, health checker |
| `internal/models` | shared request/response types |
| `internal/store` | SQLite migrations |

### Data flow

```
Browser keystroke → WS text frame {type:"input"} → switchboard proxy
                                                   → loom agent WS handler
                                                   → termInputCh → p.Write()

PTY output → broadcaster.Send() → loom agent WS
           → switchboard proxy → browser xterm.js
           → recorder.WriteOutput() → SQLite messages table
```

All browser→loom WebSocket messages are **text JSON** with a `type` field (`"input"`, `"resize"`). Loom→browser frames are **binary** (raw PTY bytes).

### WebSocket proxy (switchboard)

`handleProxyWS` in `internal/switchboard/server.go`:
- Dials loom's `/ws` as upstream
- Main goroutine: upstream → client (PTY output)
- Background goroutine: client → upstream (keyboard + resize)

Gorilla/websocket rule: one concurrent reader and one concurrent writer per connection. The proxy respects this — separate goroutines for read and write on each side.

### Headless mode (loom)

When loom's stdin is not a terminal (`term.IsTerminal` returns false), `pty.go` skips `MakeRaw` and local stdin forwarding. PTY defaults to 220×50 until the browser sends its real size via a resize message. This is the mode used when switchboard launches loom as a subprocess.

### Conversation recording

`internal/loom/recorder.go` reconstructs messages from raw byte streams:
- Buffers keystrokes, handles backspace and CSI escape sequences
- Saves a user message on Enter; saves the assistant message (stripped of ANSI) when the next Enter arrives
- Web-injected messages are intercepted in `cmd/loom/main.go` before reaching the PTY so they are also recorded

## Key conventions

- Port `1804` — Jacquard loom patent year, default for both loom `-switchboard` flag and switchboard `-addr`
- SQLite only — no external services; WAL mode enabled so both processes share the same file safely
- `INSERT OR IGNORE` — SQLite syntax, not `INSERT IGNORE` (MySQL)
- Go 1.22 routing patterns — `mux.HandleFunc("GET /path/{id}", handler)`
- All switchboard API routes have CORS headers (`Access-Control-Allow-Origin: *`) so Periscope can query any node cross-origin
