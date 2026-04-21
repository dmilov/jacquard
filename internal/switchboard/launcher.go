package switchboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Launcher spawns and tracks loom child processes started from the web UI.
type Launcher struct {
	mu        sync.Mutex
	processes map[string]*exec.Cmd
	loomBin   string
	db        *DB
}

func NewLauncher(db *DB) *Launcher {
	self, _ := os.Executable()
	bin := filepath.Join(filepath.Dir(self), "loom")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	return &Launcher{
		processes: make(map[string]*exec.Cmd),
		loomBin:   bin,
		db:        db,
	}
}

// Launch starts loom as a headless subprocess and records it in the DB so it
// can be re-launched after a switchboard restart.
// workDir sets the child's working directory; empty means inherit switchboard's cwd.
func (l *Launcher) Launch(loomID, switchboardURL, name, workDir string, args []string) error {
	command := joinArgs(args)

	loomArgs := []string{
		"-id", loomID,
		"-switchboard", switchboardURL,
		"-name", name,
		"--",
	}
	loomArgs = append(loomArgs, args...)

	cmd := exec.Command(l.loomBin, loomArgs...)
	cmd.Stdin = nil        // headless — no local terminal
	cmd.Stdout = nil       // PTY output goes to WebSocket, not here
	cmd.Stderr = os.Stderr // log lines visible in switchboard console
	if workDir != "" {
		cmd.Dir = workDir
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec loom: %w", err)
	}

	l.mu.Lock()
	l.processes[loomID] = cmd
	l.mu.Unlock()

	// Persist so switchboard can re-launch after restart.
	l.db.SaveLaunchedLoom(context.Background(), LaunchedLoom{ //nolint:errcheck
		ID:        loomID,
		Name:      name,
		Command:   command,
		WorkDir:   workDir,
		CreatedAt: time.Now().UTC(),
	})

	go func() {
		cmd.Wait() //nolint:errcheck
		l.mu.Lock()
		delete(l.processes, loomID)
		l.mu.Unlock()
		l.db.DeleteLaunchedLoom(context.Background(), loomID) //nolint:errcheck
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

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}
