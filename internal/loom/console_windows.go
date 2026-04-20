//go:build windows

package loom

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVTOutput turns on ENABLE_VIRTUAL_TERMINAL_PROCESSING for stdout so
// that ANSI escape sequences from the child process render correctly in the
// parent Windows console.
func enableVTOutput() {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return
	}
	windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) //nolint:errcheck
}
