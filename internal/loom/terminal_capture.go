package loom

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxCaptureScrollback = 4000

type terminalCapture struct {
	cols, rows int
	row, col   int
	lines      [][]rune
	scrollback []string

	escState int
	csi      strings.Builder
	osc      strings.Builder
}

func newTerminalCapture(cols, rows int) *terminalCapture {
	t := &terminalCapture{}
	t.Resize(cols, rows)
	return t
}

// CurrentLine returns the text on the line the cursor currently occupies.
func (t *terminalCapture) CurrentLine() string {
	if t.row < 0 || t.row >= len(t.lines) {
		return ""
	}
	return trimRightRunes(t.lines[t.row])
}

func (t *terminalCapture) Reset() {
	cols, rows := t.cols, t.rows
	*t = terminalCapture{}
	t.Resize(cols, rows)
}

func (t *terminalCapture) Resize(cols, rows int) {
	if cols <= 0 {
		cols = 80
	}
	if rows <= 0 {
		rows = 24
	}
	t.cols = cols
	t.rows = rows
	if len(t.lines) == 0 {
		t.lines = make([][]rune, rows)
	} else {
		for len(t.lines) < rows {
			t.lines = append(t.lines, nil)
		}
		if len(t.lines) > rows {
			for _, line := range t.lines[:len(t.lines)-rows] {
				t.pushScrollback(line)
			}
			t.lines = t.lines[len(t.lines)-rows:]
		}
	}
	if t.row >= rows {
		t.row = rows - 1
	}
	if t.col >= cols {
		t.col = cols - 1
	}
}

func (t *terminalCapture) Write(b []byte) {
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			b = b[1:]
			continue
		}
		b = b[size:]
		t.writeRune(r)
	}
}

func (t *terminalCapture) Text() string {
	var out []string
	for _, line := range t.scrollback {
		out = appendNonDuplicate(out, line)
	}
	for _, line := range t.lines {
		out = appendNonDuplicate(out, trimRightRunes(line))
	}

	out = trimEmptyEdges(out)
	out = dropPromptEcho(out)
	if len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func (t *terminalCapture) writeRune(r rune) {
	switch t.escState {
	case 1:
		t.handleEscape(r)
		return
	case 2:
		if r >= 0x40 && r <= 0x7e {
			t.handleCSI(t.csi.String(), r)
			t.csi.Reset()
			t.escState = 0
		} else {
			t.csi.WriteRune(r)
		}
		return
	case 3:
		if r == '\a' {
			t.osc.Reset()
			t.escState = 0
			return
		}
		t.osc.WriteRune(r)
		return
	case 4:
		if r == '\\' {
			t.osc.Reset()
		}
		t.escState = 0
		return
	}

	switch r {
	case '\x1b':
		t.escState = 1
	case '\r':
		t.col = 0
	case '\n':
		t.newline()
	case '\b', '\x7f':
		if t.col > 0 {
			t.col--
		}
	case '\t':
		next := ((t.col / 8) + 1) * 8
		for t.col < next {
			t.putRune(' ')
		}
	default:
		if r >= 0x20 {
			t.putRune(r)
		}
	}
}

func (t *terminalCapture) handleEscape(r rune) {
	switch r {
	case '[':
		t.csi.Reset()
		t.escState = 2
	case ']':
		t.osc.Reset()
		t.escState = 3
	case 'c':
		t.clearScreen()
		t.row, t.col = 0, 0
		t.escState = 0
	default:
		t.escState = 0
	}
}

func (t *terminalCapture) handleCSI(raw string, final rune) {
	params := parseCSIParams(raw)
	switch final {
	case 'A':
		t.row -= firstParam(params, 1)
	case 'B':
		t.row += firstParam(params, 1)
	case 'C':
		t.col += firstParam(params, 1)
	case 'D':
		t.col -= firstParam(params, 1)
	case 'E':
		t.row += firstParam(params, 1)
		t.col = 0
	case 'F':
		t.row -= firstParam(params, 1)
		t.col = 0
	case 'G':
		t.col = firstParam(params, 1) - 1
	case 'H', 'f':
		t.row = paramAt(params, 0, 1) - 1
		t.col = paramAt(params, 1, 1) - 1
	case 'J':
		t.clearDisplay(firstParam(params, 0))
	case 'K':
		t.clearLine(firstParam(params, 0))
	case 'S':
		for i := 0; i < firstParam(params, 1); i++ {
			t.scrollUp()
		}
	case 'm':
		// SGR only changes style; recorded history cares about text.
	case 'h', 'l':
		// Mode changes are intentionally ignored. Capturing alternate-screen
		// buffers would preserve CLI chrome, which is exactly what history
		// should avoid.
	}
	t.clampCursor()
}

func (t *terminalCapture) putRune(r rune) {
	if t.row < 0 {
		t.row = 0
	}
	if t.row >= t.rows {
		t.scrollUp()
	}
	if t.col >= t.cols {
		t.newline()
	}
	for len(t.lines[t.row]) <= t.col {
		t.lines[t.row] = append(t.lines[t.row], ' ')
	}
	t.lines[t.row][t.col] = r
	t.col++
}

func (t *terminalCapture) newline() {
	t.row++
	t.col = 0
	if t.row >= t.rows {
		t.scrollUp()
	}
}

func (t *terminalCapture) scrollUp() {
	if len(t.lines) == 0 {
		return
	}
	t.pushScrollback(t.lines[0])
	copy(t.lines, t.lines[1:])
	t.lines[len(t.lines)-1] = nil
	t.row = t.rows - 1
}

func (t *terminalCapture) pushScrollback(line []rune) {
	text := trimRightRunes(line)
	if text != "" {
		t.scrollback = append(t.scrollback, text)
		if len(t.scrollback) > maxCaptureScrollback {
			t.scrollback = t.scrollback[len(t.scrollback)-maxCaptureScrollback:]
		}
	}
}

func (t *terminalCapture) clearScreen() {
	for i := range t.lines {
		t.lines[i] = nil
	}
}

func (t *terminalCapture) clearDisplay(mode int) {
	switch mode {
	case 2, 3:
		t.clearScreen()
		if mode == 3 {
			t.scrollback = nil
		}
	case 1:
		for i := 0; i < t.row && i < len(t.lines); i++ {
			t.lines[i] = nil
		}
		if t.row >= 0 && t.row < len(t.lines) {
			t.lines[t.row] = keepSuffix(t.lines[t.row], t.col)
		}
	default:
		if t.row >= 0 && t.row < len(t.lines) {
			t.lines[t.row] = keepPrefix(t.lines[t.row], t.col)
		}
		for i := t.row + 1; i < len(t.lines); i++ {
			t.lines[i] = nil
		}
	}
}

func (t *terminalCapture) clearLine(mode int) {
	if t.row < 0 || t.row >= len(t.lines) {
		return
	}
	switch mode {
	case 1:
		t.lines[t.row] = keepSuffix(t.lines[t.row], t.col)
	case 2:
		t.lines[t.row] = nil
	default:
		t.lines[t.row] = keepPrefix(t.lines[t.row], t.col)
	}
}

func (t *terminalCapture) clampCursor() {
	if t.row < 0 {
		t.row = 0
	}
	if t.col < 0 {
		t.col = 0
	}
	if t.row >= t.rows {
		t.row = t.rows - 1
	}
	if t.col >= t.cols {
		t.col = t.cols - 1
	}
}

func parseCSIParams(raw string) []int {
	raw = strings.TrimLeft(raw, "?=>")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	params := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			params = append(params, 0)
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			params = append(params, 0)
			continue
		}
		params = append(params, n)
	}
	return params
}

func firstParam(params []int, fallback int) int {
	return paramAt(params, 0, fallback)
}

func paramAt(params []int, idx, fallback int) int {
	if idx >= len(params) || params[idx] == 0 {
		return fallback
	}
	return params[idx]
}

func keepPrefix(line []rune, col int) []rune {
	if col < 0 {
		return nil
	}
	if col >= len(line) {
		return line
	}
	return line[:col]
}

func keepSuffix(line []rune, col int) []rune {
	if col < 0 {
		return line
	}
	if col >= len(line) {
		return nil
	}
	for i := 0; i <= col && i < len(line); i++ {
		line[i] = ' '
	}
	return line
}

func trimRightRunes(line []rune) string {
	return strings.TrimRight(string(line), " \t")
}

func appendNonDuplicate(lines []string, line string) []string {
	if line == "" {
		lines = append(lines, "")
		return lines
	}
	if len(lines) > 0 && lines[len(lines)-1] == line {
		return lines
	}
	return append(lines, line)
}

func trimEmptyEdges(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func dropPromptEcho(lines []string) []string {
	for len(lines) > 0 {
		trimmed := strings.TrimSpace(lines[0])
		if strings.HasPrefix(trimmed, "> ") || strings.HasPrefix(trimmed, "$ ") {
			lines = lines[1:]
			continue
		}
		break
	}
	return lines
}
