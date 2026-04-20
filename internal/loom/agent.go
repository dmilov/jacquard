package loom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/dmilov/jacquard/internal/models"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Agent runs the Loom's local HTTP server, registers with Switchboard, and
// exposes WebSocket output streaming and message injection.
type Agent struct {
	info        models.LoomInfo
	switchboard string
	broadcaster *Broadcaster
	injectCh    chan string
	server      *http.Server
}

func NewAgent(info models.LoomInfo, switchboardURL string) *Agent {
	return &Agent{
		info:        info,
		switchboard: switchboardURL,
		broadcaster: NewBroadcaster(),
		injectCh:    make(chan string, 16),
	}
}

func (a *Agent) Broadcaster() *Broadcaster { return a.broadcaster }
func (a *Agent) InjectCh() <-chan string    { return a.injectCh }

// Start binds to a random local port, starts the HTTP server, and registers
// with the Switchboard.
func (a *Agent) Start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	a.info.Address = fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /inject", a.handleInject)
	mux.HandleFunc("GET /ws", a.handleWS)

	a.server = &http.Server{Handler: mux}
	go a.server.Serve(ln) //nolint:errcheck

	return a.register()
}

func (a *Agent) Shutdown() {
	a.deregister()
	close(a.injectCh)
	a.broadcaster.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a.server.Shutdown(ctx) //nolint:errcheck
}

func (a *Agent) register() error {
	body, _ := json.Marshal(models.RegisterRequest{Loom: a.info})
	resp, err := http.Post(a.switchboard+"/api/looms/register", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("register with switchboard: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (a *Agent) deregister() {
	req, _ := http.NewRequest(http.MethodDelete, a.switchboard+"/api/looms/"+a.info.ID, nil)
	c := &http.Client{Timeout: 3 * time.Second}
	if resp, err := c.Do(req); err == nil {
		resp.Body.Close()
	}
}

func (a *Agent) handleInject(w http.ResponseWriter, r *http.Request) {
	var req models.InjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	select {
	case a.injectCh <- req.Message:
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "inject buffer full", http.StatusTooManyRequests)
	}
}

func (a *Agent) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ch := a.broadcaster.Subscribe()
	defer a.broadcaster.Unsubscribe(ch)

	done := r.Context().Done()
	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
