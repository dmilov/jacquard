package loom

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"time"
)

// Recorder reconstructs user/assistant messages from raw PTY streams and
// persists them to SQLite. Message boundary: terminal output after a user
// Enter is rendered through a small virtual terminal and saved when the next
// user Enter arrives.
type Recorder struct {
	db             *sql.DB
	conversationID string

	mu           sync.Mutex
	inputBuf     []byte
	outputScreen *terminalCapture
	sequence     int
	waitingInput bool
	escState     int // 0=normal 1=got-ESC 2=in-CSI
}

func NewRecorder(db *sql.DB, conversationID string) *Recorder {
	return &Recorder{
		db:             db,
		conversationID: conversationID,
		outputScreen:   newTerminalCapture(220, 50),
		waitingInput:   true,
	}
}

// WriteInput receives raw bytes typed by the user (before PTY echo).
// Handles ANSI escape sequences so arrow keys / function keys don't corrupt
// the recorded text.
func (r *Recorder) WriteInput(b []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range b {
		switch r.escState {
		case 1: // after ESC
			if c == '[' {
				r.escState = 2 // entering CSI sequence
			} else {
				r.escState = 0 // single-char escape, skip it
			}
			continue
		case 2: // inside CSI — skip until final byte (0x40–0x7e)
			if c >= 0x40 && c <= 0x7e {
				r.escState = 0
			}
			continue
		}
		// normal state
		switch c {
		case 0x1b: // ESC — start of escape sequence
			r.escState = 1
		case 0x7f, '\b': // DEL / backspace
			if len(r.inputBuf) > 0 {
				r.inputBuf = r.inputBuf[:len(r.inputBuf)-1]
			}
		case '\r', '\n':
			msg := strings.TrimSpace(string(r.inputBuf))
			r.inputBuf = r.inputBuf[:0]
			if msg == "" {
				continue
			}
			if !r.waitingInput {
				r.flushOutput()
			}
			r.waitingInput = false
			r.outputScreen.Reset()
			r.persist("user", msg)
		default:
			if c >= 0x20 {
				r.inputBuf = append(r.inputBuf, c)
			}
		}
	}
}

// WriteOutput receives raw bytes from the PTY (CLI output).
func (r *Recorder) WriteOutput(b []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.waitingInput {
		r.outputScreen.Write(b)
	}
}

// Resize keeps the recorder's virtual terminal close to the real PTY size.
func (r *Recorder) Resize(cols, rows int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.outputScreen.Resize(cols, rows)
}

// Flush saves any buffered assistant output — call on process exit.
func (r *Recorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.waitingInput {
		r.flushOutput()
	}
}

func (r *Recorder) flushOutput() {
	clean := r.outputScreen.Text()
	r.outputScreen.Reset()
	r.waitingInput = true
	if clean == "" {
		return
	}
	r.persist("assistant", clean)
}

func (r *Recorder) persist(role, content string) {
	r.sequence++
	seq := r.sequence
	convID := r.conversationID
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = r.db.ExecContext(ctx,
			`INSERT INTO messages (conversation_id, role, content, sequence, created_at)
			 VALUES (?, ?, ?, ?, ?)`,
			convID, role, content, seq, time.Now().UTC(),
		)
	}()
}
