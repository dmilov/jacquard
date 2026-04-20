package loom

import (
	"fmt"
	"io"
	"os"

	gopty "github.com/aymanbagabas/go-pty"
	"golang.org/x/term"
)

// Run spawns args in a PTY with full terminal passthrough. inputTee receives
// every byte the user types; outputTee receives every byte the CLI outputs.
// injectCh delivers messages to write to the PTY as if the user typed them.
// resizeCh delivers {cols, rows} pairs from browser clients.
// Blocks until the child process exits.
func Run(args []string, injectCh <-chan string, resizeCh <-chan [2]int, inputTee io.Writer, outputTee io.Writer) error {
	enableVTOutput() // Windows: ensure ANSI sequences render in parent console

	p, err := gopty.New()
	if err != nil {
		return fmt.Errorf("pty.New: %w", err)
	}
	defer p.Close()

	// Headless mode: stdin is not a real terminal (e.g. launched by switchboard).
	// Skip MakeRaw and local stdin forwarding; browser drives everything.
	interactive := term.IsTerminal(int(os.Stdin.Fd()))

	if interactive {
		if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil && w > 0 {
			p.Resize(w, h) //nolint:errcheck
		}
	} else {
		p.Resize(220, 50) //nolint:errcheck // sensible default until browser sends its size
	}

	cmd := p.Command(args[0], args[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pty.Start: %w", err)
	}

	if interactive {
		oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("term.MakeRaw: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), oldState)
	}

	// Apply resize requests from browser clients.
	if resizeCh != nil {
		go func() {
			for dim := range resizeCh {
				p.Resize(dim[0], dim[1]) //nolint:errcheck
			}
		}()
	}

	// stdin → PTY (tee'd to recorder input side); skipped in headless mode.
	if interactive {
		go func() {
			dst := io.Writer(p)
			if inputTee != nil {
				dst = io.MultiWriter(p, inputTee)
			}
			io.Copy(dst, os.Stdin) //nolint:errcheck
		}()
	}

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

