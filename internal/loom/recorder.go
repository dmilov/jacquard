package loom

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ansiRe = regexp.MustCompile(`\x1b(?:\[[0-9;]*[A-Za-z]|\][^\x07]*\x07|[()][AB0-9]|.)`)

func stripANSI(s string) string {
	return strings.TrimSpace(ansiRe.ReplaceAllString(s, ""))
}

// Recorder reconstructs user/assistant messages from raw PTY streams and
// persists them to MySQL. Message boundary: everything output between two
// user Enter presses is saved as one assistant message.
type Recorder struct {
	db             *sql.DB
	conversationID string

	mu           sync.Mutex
	inputBuf     []byte
	outputBuf    strings.Builder
	sequence     int
	waitingInput bool
}

func NewRecorder(db *sql.DB, conversationID string) *Recorder {
	return &Recorder{
		db:             db,
		conversationID: conversationID,
		waitingInput:   true,
	}
}

// WriteInput receives raw bytes typed by the user (before PTY echo).
func (r *Recorder) WriteInput(b []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range b {
		switch c {
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
		r.outputBuf.Write(b)
	}
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
	raw := r.outputBuf.String()
	r.outputBuf.Reset()
	r.waitingInput = true
	clean := stripANSI(raw)
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
