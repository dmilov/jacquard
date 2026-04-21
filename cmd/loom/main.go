package main

import (
	"flag"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/dmilov/jacquard/internal/loom"
	"github.com/dmilov/jacquard/internal/models"
)

func main() {
	hostname, _ := os.Hostname()

	switchboardURL := flag.String("switchboard", "http://localhost:1804", "Switchboard URL")
	nodeID := flag.String("node", hostname, "Node identifier")
	name := flag.String("name", "", "Display name for this loom (defaults to command)")
	idFlag := flag.String("id", "", "Loom ID (generated if empty)")
	flag.Parse()

	_ = nodeID // kept for future use

	args := flag.Args()
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		log.Fatal("usage: loom [flags] -- <command> [args...]")
	}

	convID := uuid.New().String()
	loomID := *idFlag
	if loomID == "" {
		loomID = uuid.New().String()
	}
	command := strings.Join(args, " ")
	loomName := *name
	if loomName == "" {
		loomName = command
	}
	now := time.Now().UTC()

	recorder := loom.NewRecorder(convID)

	info := models.LoomInfo{
		ID:             loomID,
		ConversationID: convID,
		Name:           loomName,
		Command:        command,
		StartedAt:      now,
	}
	agent := loom.NewAgent(info, *switchboardURL, recorder)
	if err := agent.Start(); err != nil {
		log.Printf("warn: switchboard registration failed: %v", err)
	}
	defer agent.Shutdown()

	recorder.SetNeedsInputCallback(agent.SetNeedsInput)

	outputTee := io.MultiWriter(
		writerFunc(recorder.WriteOutput),
		writerFunc(agent.Broadcaster().Send),
	)
	inputTee := writerFunc(recorder.WriteInput)

	// Intercept inject channel so web-injected messages are also recorded.
	injectCh := make(chan string, 16)
	go func() {
		defer close(injectCh)
		for msg := range agent.InjectCh() {
			recorder.WriteInput([]byte(msg + "\r"))
			injectCh <- msg
		}
	}()

	resizeCh := make(chan [2]int, 16)
	go func() {
		defer close(resizeCh)
		for dim := range agent.ResizeCh() {
			recorder.Resize(dim[0], dim[1])
			resizeCh <- dim
		}
	}()

	if err := loom.Run(args, injectCh, resizeCh, agent.TermInputCh(), inputTee, outputTee); err != nil {
		log.Printf("process exited: %v", err)
	}

	recorder.Flush()
}

type writerFunc func([]byte)

func (f writerFunc) Write(p []byte) (int, error) {
	f(p)
	return len(p), nil
}
