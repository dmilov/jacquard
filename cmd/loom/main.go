package main

import (
	"database/sql"
	"flag"
	"io"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"github.com/dmilov/jacquard/internal/loom"
	"github.com/dmilov/jacquard/internal/models"
)

func main() {
	hostname, _ := os.Hostname()

	switchboardURL := flag.String("switchboard", "http://localhost:8080", "Switchboard URL")
	nodeID         := flag.String("node", hostname, "Node identifier")
	dsn            := flag.String("dsn", "", "MySQL DSN (user:pass@tcp(host:port)/jacquard?parseTime=true)")
	flag.Parse()

	args := flag.Args()
	// Strip leading "--" separator if present
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		log.Fatal("usage: loom [flags] -- <command> [args...]")
	}

	if *dsn == "" {
		log.Fatal("-dsn is required")
	}

	db, err := sql.Open("mysql", *dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	convID := uuid.New().String()
	loomID := uuid.New().String()
	command := strings.Join(args, " ")
	now := time.Now().UTC()

	// Persist the conversation record
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
		Command:        command,
		StartedAt:      now,
	}
	agent := loom.NewAgent(info, *switchboardURL)
	if err := agent.Start(); err != nil {
		// Non-fatal: continue without Switchboard registration
		log.Printf("warn: switchboard registration failed: %v", err)
	}
	defer agent.Shutdown()

	// Wire output: PTY → stdout + recorder + broadcaster
	outputTee := io.MultiWriter(
		writerFunc(recorder.WriteOutput),
		writerFunc(agent.Broadcaster().Send),
	)
	// Wire input: stdin → PTY + recorder
	inputTee := writerFunc(recorder.WriteInput)

	if err := loom.Run(args, agent.InjectCh(), inputTee, outputTee); err != nil {
		log.Printf("process exited: %v", err)
	}

	recorder.Flush()

	_, _ = db.Exec(
		`UPDATE conversations SET ended_at=? WHERE id=?`,
		time.Now().UTC(), convID,
	)
}

// writerFunc adapts a func([]byte) to io.Writer.
type writerFunc func([]byte)

func (f writerFunc) Write(p []byte) (int, error) {
	f(p)
	return len(p), nil
}
