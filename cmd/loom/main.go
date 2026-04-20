package main

import (
	"database/sql"
	"flag"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/dmilov/jacquard/internal/loom"
	"github.com/dmilov/jacquard/internal/models"
	"github.com/dmilov/jacquard/internal/store"
)

func main() {
	hostname, _ := os.Hostname()

	switchboardURL := flag.String("switchboard", "http://localhost:1804", "Switchboard URL")
	nodeID         := flag.String("node", hostname, "Node identifier")
	dbPath         := flag.String("db", "jacquard.db", "SQLite database file path")
	name           := flag.String("name", "", "Display name for this loom (defaults to command)")
	flag.Parse()

	args := flag.Args()
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		log.Fatal("usage: loom [flags] -- <command> [args...]")
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	db.Exec("PRAGMA journal_mode=WAL")  //nolint:errcheck
	db.Exec("PRAGMA foreign_keys=ON")   //nolint:errcheck

	if err := store.Migrate(db); err != nil {
		log.Fatalf("migrate db: %v", err)
	}

	convID   := uuid.New().String()
	loomID   := uuid.New().String()
	command  := strings.Join(args, " ")
	loomName := *name
	if loomName == "" {
		loomName = command
	}
	now := time.Now().UTC()

	_, err = db.Exec(
		`INSERT INTO conversations (id, node_id, command, started_at) VALUES (?, ?, ?, ?)`,
		convID, *nodeID, command, now,
	)
	if err != nil {
		log.Fatalf("create conversation: %v", err)
	}

	recorder := loom.NewRecorder(db, convID)

	info := models.LoomInfo{
		ID:             loomID,
		ConversationID: convID,
		Name:           loomName,
		Command:        command,
		StartedAt:      now,
	}
	agent := loom.NewAgent(info, *switchboardURL)
	if err := agent.Start(); err != nil {
		log.Printf("warn: switchboard registration failed: %v", err)
	}
	defer agent.Shutdown()

	outputTee := io.MultiWriter(
		writerFunc(recorder.WriteOutput),
		writerFunc(agent.Broadcaster().Send),
	)
	inputTee := writerFunc(recorder.WriteInput)

	if err := loom.Run(args, agent.InjectCh(), agent.ResizeCh(), inputTee, outputTee); err != nil {
		log.Printf("process exited: %v", err)
	}

	recorder.Flush()
	_, _ = db.Exec(`UPDATE conversations SET ended_at=? WHERE id=?`, time.Now().UTC(), convID)
}

type writerFunc func([]byte)

func (f writerFunc) Write(p []byte) (int, error) {
	f(p)
	return len(p), nil
}
