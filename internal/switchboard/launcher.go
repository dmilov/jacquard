package switchboard

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// Launcher spawns and tracks loom child processes started from the web UI.
type Launcher struct {
	mu        sync.Mutex
	processes map[string]*exec.Cmd
	loomBin   string
}

func NewLauncher() *Launcher {
	self, _ := os.Executable()
	bin := filepath.Join(filepath.Dir(self), "loom")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	return &Launcher{
		processes: make(map[string]*exec.Cmd),
		loomBin:   bin,
	}
}

// Launch starts loom as a headless subprocess. dbPath should match the
// switchboard's own database so both share the same conversation records.
func (l *Launcher) Launch(loomID, switchboardURL, name, dbPath string, args []string) error {
	loomArgs := []string{
		"-id", loomID,
		"-switchboard", switchboardURL,
		"-name", name,
		"-db", dbPath,
		"--",
	}
	loomArgs = append(loomArgs, args...)

	cmd := exec.Command(l.loomBin, loomArgs...)
	cmd.Stdin = nil        // headless — no local terminal
	cmd.Stdout = nil       // PTY output goes to WebSocket, not here
	cmd.Stderr = os.Stderr // log lines visible in switchboard console

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec loom: %w", err)
	}

	l.mu.Lock()
	l.processes[loomID] = cmd
	l.mu.Unlock()

	go func() {
		cmd.Wait() //nolint:errcheck
		l.mu.Lock()
		delete(l.processes, loomID)
		l.mu.Unlock()
	}()
	return nil
}

// Kill terminates a loom process that was started by Launch.
// Returns false if the loom was not launched by this switchboard instance.
func (l *Launcher) Kill(loomID string) bool {
	l.mu.Lock()
	cmd, ok := l.processes[loomID]
	l.mu.Unlock()
	if !ok {
		return false
	}
	cmd.Process.Kill() //nolint:errcheck
	return true
}
