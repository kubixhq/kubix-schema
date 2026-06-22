package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/kubixhq/kubix-schema/internal/config"
	"github.com/kubixhq/kubix-schema/internal/db"
	"github.com/kubixhq/kubix-schema/internal/handler"
	"github.com/kubixhq/kubix-schema/internal/schema"
)

func main() {
	cfg := config.Load()

	database, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("cannot connect to database: %v", err)
	}
	defer database.Close()

	if err := os.MkdirAll(cfg.SnapshotDir, 0755); err != nil {
		log.Printf("warning: cannot create snapshot dir: %v", err)
	}

	tool := schema.DetectMigrationTool(database, cfg.MigrationTool)
	log.Printf("migration tool: %s", tool)

	if err := schema.CaptureStartupSnapshot(database, tool, cfg.SnapshotDir); err != nil {
		log.Printf("warning: startup snapshot failed: %v", err)
	}

	h := handler.New(database, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/schema/erd", h.ERD)
	mux.HandleFunc("GET /api/schema/migrations", h.Migrations)
	mux.HandleFunc("GET /api/schema/diff", h.Diff)

	addr := fmt.Sprintf(":%d", cfg.ServerPort)
	log.Printf("kubix-schema listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
