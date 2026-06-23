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

	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

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
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /api/schema/erd", h.ERD)
	mux.HandleFunc("GET /api/schema/migrations", h.Migrations)
	mux.HandleFunc("GET /api/schema/diff", h.Diff)
	mux.HandleFunc("GET /api/schema/snapshots", h.ListSnapshots)
	mux.HandleFunc("POST /api/schema/snapshots", h.SaveSnapshot)
	mux.HandleFunc("GET /api/schema/risk", h.Risk)

	addr := fmt.Sprintf(":%d", cfg.ServerPort)
	log.Printf("kubix-schema listening on %s", addr)
	if err := http.ListenAndServe(addr, cors(mux)); err != nil {
		log.Fatal(err)
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
