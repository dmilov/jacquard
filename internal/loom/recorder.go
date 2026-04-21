package loom

import (
	"strings"
	"sync"
	"time"

	"github.com/dmilov/jacquard/internal/models"
)

// Recorder reconstructs user/assistant messages from raw PTY streams and
// keeps them in memory. Message boundary: terminal output after a user
// Enter is rendered through a small virtual terminal and saved when the next
// user Enter arrives.
type Recorder struct {
	conversationID string

	mu           sync.Mutex
	inputBuf     []byte
	outputScreen *terminalCapture
	sequence     int
	waitingInput bool
	escState     int // 0=normal 1=got-ESC 2=in-CSI

	messages []models.Message

	onNeedsInput    func(bool)
	needsInputState bool
	lastPromptCheck time.Time
}

func NewRecorder(conversationID string) *Recorder {
	return &Recorder{
		conversationID: conversationID,
		outputScreen:   newTerminalCapture(220, 50),
		waitingInput:   true,
	}
}

// SetNeedsInputCallback registers a function called whenever the recorder's
// assessment of whether the CLI is waiting for user input changes.
func (r *Recorder) SetNeedsInputCallback(fn func(bool)) {
	r.mu.Lock()
	r.onNeedsInput = fn
	r.mu.Unlock()
}

// Messages returns a snapshot of all recorded messages.
func (r *Recorder) Messages() []models.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]models.Message, len(r.messages))
	copy(out, r.messages)
	return out
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
			if r.needsInputState && r.onNeedsInput != nil {
				r.needsInputState = false
				go r.onNeedsInput(false)
			}
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
	// Rate-limited prompt detection: check at most every 150 ms.
	if r.onNeedsInput != nil && time.Since(r.lastPromptCheck) > 150*time.Millisecond {
		r.lastPromptCheck = time.Now()
		detected := looksLikePrompt(r.outputScreen.CurrentLine())
		if detected != r.needsInputState {
			r.needsInputState = detected
			go r.onNeedsInput(detected)
		}
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
	raw := r.outputScreen.Text()
	r.outputScreen.Reset()
	r.waitingInput = true
	clean := extractFinalResponse(raw)
	if clean == "" {
		return
	}
	r.persist("assistant", clean)
}

// looksLikePrompt returns true when a terminal line appears to be a CLI
// asking the user for confirmation or a yes/no answer.
func looksLikePrompt(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	low := strings.ToLower(t)
	return strings.Contains(low, "(y/n)") ||
		strings.Contains(low, "[y/n]") ||
		strings.Contains(low, "yes/no") ||
		strings.HasPrefix(t, "❯") || // Claude CLI selection menu
		strings.HasPrefix(t, "›")    // alternate selection sigil
}

// extractFinalResponse strips tool-call blocks (●/│/└ lines) and shell prompt
// lines, returning only the final prose response from the CLI.
func extractFinalResponse(text string) string {
	lines := strings.Split(text, "\n")

	// Find the last line that is part of a tool-call block.
	lastToolLine := -1
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "●") || strings.HasPrefix(t, "│") ||
			strings.HasPrefix(t, "└") || strings.HasPrefix(t, "┌") ||
			strings.HasPrefix(t, "├") {
			lastToolLine = i
		}
	}

	start := lastToolLine + 1
	if start >= len(lines) {
		return ""
	}
	tail := lines[start:]

	for len(tail) > 0 {
		t := strings.TrimSpace(tail[0])
		if isPromptLine(t) || t == "" {
			tail = tail[1:]
		} else {
			break
		}
	}

	for len(tail) > 0 {
		t := strings.TrimSpace(tail[len(tail)-1])
		if isPromptLine(t) || t == "" {
			tail = tail[:len(tail)-1]
		} else {
			break
		}
	}

	return strings.TrimSpace(strings.Join(tail, "\n"))
}

// isPromptLine returns true for lines that look like a shell prompt or
// decorative separator rather than response content.
func isPromptLine(s string) bool {
	if s == "" {
		return false
	}
	allDeco := true
	for _, r := range s {
		if r != '─' && r != '-' && r != '═' && r != ' ' {
			allDeco = false
			break
		}
	}
	if allDeco {
		return true
	}
	return strings.HasPrefix(s, "❯") || strings.HasPrefix(s, "$") ||
		strings.HasPrefix(s, ">") || strings.HasPrefix(s, "#")
}

func (r *Recorder) persist(role, content string) {
	r.sequence++
	r.messages = append(r.messages, models.Message{
		ConversationID: r.conversationID,
		Role:           role,
		Content:        content,
		Sequence:       r.sequence,
		CreatedAt:      time.Now().UTC(),
	})
}
