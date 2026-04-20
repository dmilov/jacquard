package main

import (
	"database/sql"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"

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

	registry := switchboard.NewRegistry()
	sdb      := switchboard.NewDB(db)
	server   := switchboard.NewServer(registry, sdb, *nodeID)
	switchboard.StartHealthChecker(registry)

	subFS, err := fs.Sub(jweb.FS, "periscope")
	if err != nil {
		log.Fatalf("embed fs: %v", err)
	}

	log.Printf("Switchboard listening on %s (node: %s)", *addr, *nodeID)
	if err := http.ListenAndServe(*addr, server.Handler(http.FS(subFS))); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
