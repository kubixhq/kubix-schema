package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kubixhq/kubix-schema/internal/config"
	"github.com/kubixhq/kubix-schema/internal/schema"
)

type Handler struct {
	db  *sql.DB
	cfg config.Config
}

func New(db *sql.DB, cfg config.Config) *Handler {
	return &Handler{db: db, cfg: cfg}
}

// GET /api/schema/erd
func (h *Handler) ERD(w http.ResponseWriter, r *http.Request) {
	erd, err := schema.ExtractERD(h.db)
	if err != nil {
		h.handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, erd)
}

// GET /api/schema/migrations
func (h *Handler) Migrations(w http.ResponseWriter, r *http.Request) {
	histories, err := schema.FetchAllMigrations(h.db, h.cfg.MigrationTool)
	if err != nil {
		h.handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, histories)
}

// GET /api/schema/diff?from=v1&to=v2
func (h *Handler) Diff(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		writeError(w, http.StatusBadRequest, "from and to query parameters are required")
		return
	}

	fromERD, err := schema.LoadSnapshot(h.db, from, h.cfg.SnapshotDir)
	if err != nil {
		h.handleSnapshotError(w, err)
		return
	}
	toERD, err := schema.LoadSnapshot(h.db, to, h.cfg.SnapshotDir)
	if err != nil {
		h.handleSnapshotError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, schema.Diff(fromERD, toERD, from, to))
}

func (h *Handler) handleDBError(w http.ResponseWriter, err error) {
	if isTimeout(err) {
		writeError(w, http.StatusRequestTimeout, "database query timed out")
		return
	}
	writeError(w, http.StatusServiceUnavailable, "database unavailable")
}

func (h *Handler) handleSnapshotError(w http.ResponseWriter, err error) {
	msg := err.Error()
	if strings.Contains(msg, "snapshot not found") {
		writeError(w, http.StatusNotFound, msg)
		return
	}
	if strings.Contains(msg, "corrupt snapshot") {
		writeError(w, http.StatusInternalServerError, msg)
		return
	}
	h.handleDBError(w, err)
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "timeout") ||
		strings.Contains(s, "deadline exceeded") ||
		strings.Contains(s, "context deadline")
}
