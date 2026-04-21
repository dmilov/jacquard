package loom

import "testing"

func TestTerminalCaptureCollapsesCarriageReturnUpdates(t *testing.T) {
	cap := newTerminalCapture(80, 10)

	cap.Write([]byte("Thinking\rDone    \nFinal answer\n"))

	got := cap.Text()
	want := "Done\nFinal answer"
	if got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

func TestTerminalCaptureClearLineRemovesSpinnerText(t *testing.T) {
	cap := newTerminalCapture(80, 10)

	cap.Write([]byte("Searching...\r\x1b[2KHere is the answer.\n"))

	got := cap.Text()
	want := "Here is the answer."
	if got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

func TestTerminalCaptureCursorMovementRewritesRenderedText(t *testing.T) {
	cap := newTerminalCapture(80, 10)

	cap.Write([]byte("alpha\nbeta\n\x1b[1Agamma\n"))

	got := cap.Text()
	want := "alpha\ngamma"
	if got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}

func TestTerminalCaptureKeepsScrolledTranscript(t *testing.T) {
	cap := newTerminalCapture(80, 2)

	cap.Write([]byte("one\ntwo\nthree\n"))

	got := cap.Text()
	want := "one\ntwo\nthree"
	if got != want {
		t.Fatalf("Text() = %q, want %q", got, want)
	}
}
