package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/dmilov/jacquard/internal/store"
	"github.com/dmilov/jacquard/internal/switchboard"
	jweb "github.com/dmilov/jacquard/web"
)

func main() {
	hostname, _ := os.Hostname()

	addr   := flag.String("addr", ":1804", "Listen address")
	dbPath := flag.String("db", "jacquard.db", "SQLite database file path")
	nodeID := flag.String("node", hostname, "Node identifier")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	db.Exec("PRAGMA journal_mode=WAL") //nolint:errcheck
	db.Exec("PRAGMA foreign_keys=ON")  //nolint:errcheck

	if err := store.Migrate(db); err != nil {
		log.Fatalf("migrate db: %v", err)
	}

	// Derive switchboard's own base URL for looms to register back to.
	host := *addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	switchboardURL := fmt.Sprintf("http://%s", host)

	sdb      := switchboard.NewDB(db)
	registry := switchboard.NewRegistry()
	launcher := switchboard.NewLauncher(sdb)
	server   := switchboard.NewServer(registry, *nodeID, switchboardURL, launcher)
	switchboard.StartHealthChecker(registry)

	// Re-launch any looms that were running before this switchboard restarted.
	if prev, err := sdb.ListLaunchedLooms(context.Background()); err != nil {
		log.Printf("warn: list launched looms: %v", err)
	} else {
		for _, l := range prev {
			args := strings.Fields(l.Command)
			if err := launcher.Launch(l.ID, switchboardURL, l.Name, l.WorkDir, args); err != nil {
				log.Printf("warn: relaunch %q: %v", l.Name, err)
			} else {
				log.Printf("relaunched loom %q (%s)", l.Name, l.ID)
			}
		}
	}

	subFS, err := fs.Sub(jweb.FS, "periscope")
	if err != nil {
		log.Fatalf("embed fs: %v", err)
	}

	log.Printf("Switchboard listening on %s (node: %s)", *addr, *nodeID)
	if err := http.ListenAndServe(*addr, server.Handler(http.FS(subFS))); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
