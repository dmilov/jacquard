package main

import (
	"database/sql"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"

	_ "github.com/go-sql-driver/mysql"

	"github.com/dmilov/jacquard/internal/switchboard"
	jweb "github.com/dmilov/jacquard/web"
)

func main() {
	hostname, _ := os.Hostname()

	addr   := flag.String("addr", ":8080", "Listen address")
	dsn    := flag.String("dsn", "", "MySQL DSN (user:pass@tcp(host:port)/jacquard?parseTime=true)")
	nodeID := flag.String("node", hostname, "Node identifier")
	flag.Parse()

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

	registry := switchboard.NewRegistry()
	sdb      := switchboard.NewDB(db)
	server   := switchboard.NewServer(registry, sdb, *nodeID)

	// Serve embedded Periscope files from web/periscope/
	subFS, err := fs.Sub(jweb.FS, "periscope")
	if err != nil {
		log.Fatalf("embed fs: %v", err)
	}

	handler := server.Handler(http.FS(subFS))

	log.Printf("Switchboard listening on %s (node: %s)", *addr, *nodeID)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
