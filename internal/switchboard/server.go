package switchboard

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/dmilov/jacquard/internal/models"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	registry       *Registry
	db             *DB
	nodeID         string
	launcher       *Launcher
	switchboardURL string
	dbPath         string
}

func NewServer(registry *Registry, db *DB, nodeID, switchboardURL, dbPath string, launcher *Launcher) *Server {
	return &Server{
		registry:       registry,
		db:             db,
		nodeID:         nodeID,
		launcher:       launcher,
		switchboardURL: switchboardURL,
		dbPath:         dbPath,
	}
}

func (s *Server) Handler(webFS http.FileSystem) http.Handler {
	mux := http.NewServeMux()

	// Loom lifecycle
	mux.HandleFunc("POST /api/looms/register", s.handleRegister)
	mux.HandleFunc("DELETE /api/looms/{id}", s.handleDeregister)
	mux.HandleFunc("POST /api/looms/launch", s.handleLaunch)
	mux.HandleFunc("POST /api/looms/{id}/kill", s.handleKill)

	// Loom queries
	mux.HandleFunc("GET /api/looms", s.handleListLooms)
	mux.HandleFunc("PATCH /api/looms/{id}", s.handleRename)
	mux.HandleFunc("POST /api/looms/{id}/inject", s.handleInject)
	mux.HandleFunc("GET /api/looms/{id}/ws", s.handleProxyWS)

	// Conversation history
	mux.HandleFunc("GET /api/conversations", s.handleListConversations)
	mux.HandleFunc("GET /api/conversations/{id}/messages", s.handleGetMessages)

	// Periscope web UI
	mux.Handle("/", http.FileServer(webFS))

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.registry.Register(req.Loom)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeregister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.registry.Deregister(id)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleLaunch(w http.ResponseWriter, r *http.Request) {
	var req models.LaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Command == "" {
		http.Error(w, "command required", http.StatusBadRequest)
		return
	}
	args := strings.Fields(req.Command)
	name := req.Name
	if name == "" {
		name = req.Command
	}
	loomID := uuid.New().String()
	if err := s.launcher.Launch(loomID, s.switchboardURL, name, s.dbPath, req.WorkDir, args); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"id": loomID})
}

func (s *Server) handleKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.launcher.Kill(id) // best-effort; false just means it wasn't launched by us
	s.registry.Deregister(id)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleListLooms(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.registry.List())
}

func (s *Server) handleRename(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.registry.Rename(r.PathValue("id"), req.Name) {
		http.Error(w, "loom not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleInject(w http.ResponseWriter, r *http.Request) {
	loom, ok := s.registry.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "loom not found", http.StatusNotFound)
		return
	}
	body, _ := io.ReadAll(r.Body)
	resp, err := http.Post(loom.Address+"/inject", "application/json", strings.NewReader(string(body)))
	if err != nil {
		http.Error(w, "loom unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.WriteHeader(resp.StatusCode)
}

func (s *Server) handleProxyWS(w http.ResponseWriter, r *http.Request) {
	loom, ok := s.registry.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "loom not found", http.StatusNotFound)
		return
	}

	// Connect to Loom's WebSocket
	loomWSURL := strings.Replace(loom.Address, "http://", "ws://", 1) + "/ws"
	upstream, _, err := websocket.DefaultDialer.Dial(loomWSURL, nil)
	if err != nil {
		http.Error(w, "cannot connect to loom", http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	// Upgrade the browser connection
	client, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer client.Close()

	// Forward all client → upstream frames: text = resize, binary = keyboard input.
	go func() {
		for {
			msgType, data, err := client.ReadMessage()
			if err != nil {
				return
			}
			upstream.WriteMessage(msgType, data) //nolint:errcheck
		}
	}()

	// Forward binary frames upstream → client (PTY output).
	for {
		msgType, data, err := upstream.ReadMessage()
		if err != nil {
			return
		}
		if err := client.WriteMessage(msgType, data); err != nil {
			return
		}
	}
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node_id")
	convs, err := s.db.ListConversations(r.Context(), nodeID)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if convs == nil {
		convs = []models.Conversation{}
	}
	writeJSON(w, convs)
}

func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	msgs, err := s.db.GetMessages(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if msgs == nil {
		msgs = []models.Message{}
	}
	writeJSON(w, msgs)
}
