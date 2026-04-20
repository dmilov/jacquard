package loom

import (
	"fmt"
	"io"
	"os"
	"time"

	gopty "github.com/aymanbagabas/go-pty"
	"golang.org/x/term"
)

// Run spawns args in a PTY with full terminal passthrough. inputTee receives
// every byte the user types; outputTee receives every byte the CLI outputs.
// injectCh delivers messages to write to the PTY as if the user typed them.
// Blocks until the child process exits.
func Run(args []string, injectCh <-chan string, inputTee io.Writer, outputTee io.Writer) error {
	p, err := gopty.New()
	if err != nil {
		return fmt.Errorf("pty.New: %w", err)
	}
	defer p.Close()

	cmd := p.Command(args[0], args[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pty.Start: %w", err)
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("term.MakeRaw: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	go pollResize(p)

	// stdin → PTY (tee'd to recorder input side)
	go func() {
		dst := io.Writer(p)
		if inputTee != nil {
			dst = io.MultiWriter(p, inputTee)
		}
		io.Copy(dst, os.Stdin) //nolint:errcheck
	}()

	// injected messages → PTY
	if injectCh != nil {
		go func() {
			for msg := range injectCh {
				p.Write([]byte(msg + "\r")) //nolint:errcheck
			}
		}()
	}

	// PTY → stdout (tee'd to recorder output side + broadcaster)
	dst := io.Writer(os.Stdout)
	if outputTee != nil {
		dst = io.MultiWriter(os.Stdout, outputTee)
	}
	io.Copy(dst, p) //nolint:errcheck

	return cmd.Wait()
}

func pollResize(p gopty.Pty) {
	var lastW, lastH int
	for {
		w, h, err := term.GetSize(int(os.Stdin.Fd()))
		if err == nil && (w != lastW || h != lastH) {
			p.Resize(w, h) //nolint:errcheck
			lastW, lastH = w, h
		}
		time.Sleep(250 * time.Millisecond)
	}
}
