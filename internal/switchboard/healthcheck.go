package switchboard

import (
	"net/http"
	"time"
)

// StartHealthChecker runs a background goroutine that pings each registered
// loom's /health endpoint every 10 seconds and deregisters any that don't
// respond, so the UI reflects reality when a loom process is killed.
func StartHealthChecker(registry *Registry) {
	client := &http.Client{Timeout: 2 * time.Second}
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for range t.C {
			for _, loom := range registry.List() {
				if _, err := client.Get(loom.Address + "/health"); err != nil {
					registry.Deregister(loom.ID)
				}
			}
		}
	}()
}
