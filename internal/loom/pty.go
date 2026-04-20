package loom

import (
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// Run spawns args in a PTY with full terminal passthrough. inputTee receives
// every byte the user types; outputTee receives every byte the CLI outputs.
// injectCh delivers messages to write to the PTY as if the user typed them.
// Blocks until the child process exits.
func Run(args []string, injectCh <-chan string, inputTee io.Writer, outputTee io.Writer) error {
	cmd := exec.Command(args[0], args[1:]...)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer ptmx.Close()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	go pollResize(ptmx)

	// stdin → PTY (tee'd to recorder input side)
	go func() {
		dst := io.Writer(ptmx)
		if inputTee != nil {
			dst = io.MultiWriter(ptmx, inputTee)
		}
		io.Copy(dst, os.Stdin) //nolint:errcheck
	}()

	// injected messages → PTY
	if injectCh != nil {
		go func() {
			for msg := range injectCh {
				ptmx.WriteString(msg + "\r") //nolint:errcheck
			}
		}()
	}

	// PTY → stdout (tee'd to recorder output side + broadcaster)
	dst := io.Writer(os.Stdout)
	if outputTee != nil {
		dst = io.MultiWriter(os.Stdout, outputTee)
	}
	io.Copy(dst, ptmx) //nolint:errcheck

	return cmd.Wait()
}

func pollResize(ptmx *os.File) {
	var lastW, lastH int
	for {
		w, h, err := term.GetSize(int(os.Stdin.Fd()))
		if err == nil && (w != lastW || h != lastH) {
			_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(h), Cols: uint16(w)})
			lastW, lastH = w, h
		}
		time.Sleep(250 * time.Millisecond)
	}
}
