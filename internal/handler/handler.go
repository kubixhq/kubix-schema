package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

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
	writeJSON(w, http.StatusOK, toErdResponse(erd))
}

// GET /api/schema/migrations
func (h *Handler) Migrations(w http.ResponseWriter, r *http.Request) {
	histories, err := schema.FetchAllMigrations(h.db, h.cfg.MigrationTool)
	if err != nil {
		h.handleDBError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toMigrationsResponse(histories))
}

// --- response shaping ---

type apiColumn struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	Nullable     bool    `json:"nullable"`
	DefaultValue *string `json:"defaultValue,omitempty"`
	IsPrimaryKey bool    `json:"isPrimaryKey"`
	IsForeignKey bool    `json:"isForeignKey"`
}

type apiForeignKey struct {
	ColumnName       string `json:"columnName"`
	ReferencedTable  string `json:"referencedTable"`
	ReferencedColumn string `json:"referencedColumn"`
}

type apiTable struct {
	Name        string          `json:"name"`
	Columns     []apiColumn     `json:"columns"`
	ForeignKeys []apiForeignKey `json:"foreignKeys"`
}

type apiErdResponse struct {
	Tables          []apiTable `json:"tables"`
	TotalColumns    int        `json:"totalColumns"`
	TotalForeignKeys int       `json:"totalForeignKeys"`
}

func toErdResponse(erd *schema.ERD) apiErdResponse {
	tables := make([]apiTable, 0, len(erd.Tables))
	totalCols, totalFKs := 0, 0
	for _, t := range erd.Tables {
		pkSet := make(map[string]bool, len(t.PrimaryKeys))
		for _, pk := range t.PrimaryKeys {
			pkSet[pk] = true
		}
		fkSet := make(map[string]bool, len(t.ForeignKeys))
		for _, fk := range t.ForeignKeys {
			fkSet[fk.Column] = true
		}
		cols := make([]apiColumn, 0, len(t.Columns))
		for _, c := range t.Columns {
			cols = append(cols, apiColumn{
				Name:         c.Name,
				Type:         c.Type,
				Nullable:     c.Nullable,
				DefaultValue: c.Default,
				IsPrimaryKey: pkSet[c.Name],
				IsForeignKey: fkSet[c.Name],
			})
		}
		fks := make([]apiForeignKey, 0, len(t.ForeignKeys))
		for _, fk := range t.ForeignKeys {
			fks = append(fks, apiForeignKey{
				ColumnName:       fk.Column,
				ReferencedTable:  fk.ReferencedTable,
				ReferencedColumn: fk.ReferencedColumn,
			})
		}
		tables = append(tables, apiTable{Name: t.Name, Columns: cols, ForeignKeys: fks})
		totalCols += len(cols)
		totalFKs += len(fks)
	}
	return apiErdResponse{Tables: tables, TotalColumns: totalCols, TotalForeignKeys: totalFKs}
}

type apiMigration struct {
	Version       string `json:"version"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	AppliedAt     string `json:"appliedAt"`
	Checksum      string `json:"checksum"`
	Tool          string `json:"tool"`
	ExecutionMs   int64  `json:"executionMs"`
	Script        string `json:"script"`
	InstalledBy   string `json:"installedBy"`
}

type apiMigrationsResponse struct {
	Migrations []apiMigration `json:"migrations"`
	Tool       string         `json:"tool"`
}

func toMigrationsResponse(histories []schema.MigrationHistory) apiMigrationsResponse {
	if len(histories) == 0 {
		return apiMigrationsResponse{Migrations: []apiMigration{}, Tool: "none"}
	}
	tool := histories[0].Tool
	var all []apiMigration
	for _, h := range histories {
		for _, m := range h.Migrations {
			checksum := ""
			if m.Checksum != nil {
				checksum = *m.Checksum
			}
			all = append(all, apiMigration{
				Version:     m.Version,
				Description: m.Description,
				Status:      m.Status,
				AppliedAt:   m.AppliedAt.Format("2006-01-02T15:04:05Z07:00"),
				Checksum:    checksum,
				Tool:        h.Tool,
				ExecutionMs: m.ExecutionTime,
				Script:      m.Script,
				InstalledBy: m.InstalledBy,
			})
		}
	}
	if all == nil {
		all = []apiMigration{}
	}
	return apiMigrationsResponse{Migrations: all, Tool: tool}
}

// GET /api/schema/snapshots
func (h *Handler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	dir := h.cfg.SnapshotDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	type snapshotInfo struct {
		Name      string `json:"name"`
		CreatedAt string `json:"createdAt"`
	}
	var list []snapshotInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, _ := e.Info()
		name := strings.TrimSuffix(e.Name(), ".json")
		createdAt := ""
		if info != nil {
			createdAt = info.ModTime().UTC().Format(time.RFC3339)
		}
		list = append(list, snapshotInfo{Name: name, CreatedAt: createdAt})
	}
	if list == nil {
		list = []snapshotInfo{}
	}
	writeJSON(w, http.StatusOK, list)
}

// POST /api/schema/snapshots  { name: "v1" }
func (h *Handler) SaveSnapshot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	erd, err := schema.ExtractERD(h.db)
	if err != nil {
		h.handleDBError(w, err)
		return
	}
	if err := schema.SaveSnapshot(erd, req.Name, h.cfg.SnapshotDir); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save snapshot")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name, "status": "saved"})
}

// GET /api/schema/risk?table=users
func (h *Handler) Risk(w http.ResponseWriter, r *http.Request) {
	table := r.URL.Query().Get("table")
	if table == "" {
		writeError(w, http.StatusBadRequest, "table parameter is required")
		return
	}

	type tableStats struct {
		EstimatedRows int64  `json:"estimatedRows"`
		SizePretty    string `json:"sizePretty"`
		SizeBytes     int64  `json:"sizeBytes"`
	}
	type columnRisk struct {
		Column      string `json:"column"`
		Type        string `json:"type"`
		Nullable    bool   `json:"nullable"`
		HasDefault  bool   `json:"hasDefault"`
		RiskLevel   string `json:"riskLevel"`
		RiskReason  string `json:"riskReason"`
		EstimatedMs int64  `json:"estimatedMs"`
	}
	type riskResponse struct {
		Table       string       `json:"table"`
		Stats       tableStats   `json:"stats"`
		OverallRisk string       `json:"overallRisk"`
		Columns     []columnRisk `json:"columns"`
		Advice      []string     `json:"advice"`
	}

	var stats tableStats
	_ = h.db.QueryRowContext(r.Context(), `
		SELECT
			COALESCE(n_live_tup, 0),
			pg_size_pretty(pg_total_relation_size($1::text)),
			pg_total_relation_size($1::text)
		FROM pg_stat_user_tables
		WHERE relname = $1
	`, table).Scan(&stats.EstimatedRows, &stats.SizePretty, &stats.SizeBytes)

	if stats.SizePretty == "" {
		stats.SizePretty = "unknown"
	}

	// fetch columns with their properties
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT
			column_name,
			data_type,
			is_nullable = 'YES',
			column_default IS NOT NULL
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`, table)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query table info")
		return
	}
	defer rows.Close()

	var colRisks []columnRisk
	overallRisk := "LOW"
	var advice []string

	rowsPerMs := int64(5000) // rough estimate: 5k rows/ms for table rewrite

	for rows.Next() {
		var cr columnRisk
		if err := rows.Scan(&cr.Column, &cr.Type, &cr.Nullable, &cr.HasDefault); err != nil {
			continue
		}
		cr.RiskLevel = "LOW"
		cr.RiskReason = "Safe operation"
		cr.EstimatedMs = 0

		// NOT NULL without default on large table = full table rewrite
		if !cr.Nullable && !cr.HasDefault && stats.EstimatedRows > 10000 {
			cr.RiskLevel = "HIGH"
			cr.RiskReason = "NOT NULL without DEFAULT requires full table rewrite + ACCESS EXCLUSIVE lock"
			cr.EstimatedMs = stats.EstimatedRows / rowsPerMs
			overallRisk = "HIGH"
			advice = append(advice, "Add a DEFAULT value before applying NOT NULL constraint")
			advice = append(advice, "Consider using NOT VALID constraint first, then VALIDATE CONSTRAINT")
		} else if !cr.Nullable && !cr.HasDefault {
			cr.RiskLevel = "MEDIUM"
			cr.RiskReason = "NOT NULL without DEFAULT — safe for small tables but locks during write"
			overallRisk = max2(overallRisk, "MEDIUM")
		}

		// type changes that require rewrite
		dangerous := []string{"text", "varchar", "character varying", "integer", "bigint"}
		isDangerous := false
		for _, d := range dangerous {
			if strings.Contains(cr.Type, d) {
				isDangerous = true
				break
			}
		}
		if isDangerous && stats.EstimatedRows > 100000 {
			cr.RiskLevel = max2(cr.RiskLevel, "MEDIUM")
			cr.RiskReason = "Column type change on large table may require rewrite"
			overallRisk = max2(overallRisk, "MEDIUM")
		}

		colRisks = append(colRisks, cr)
	}

	if stats.EstimatedRows > 1000000 {
		advice = append(advice, "Table has >1M rows — consider using pg_repack or online DDL tools")
	}
	if overallRisk == "LOW" {
		advice = append(advice, "Schema looks safe to migrate — no high-risk columns detected")
	}
	if colRisks == nil {
		colRisks = []columnRisk{}
	}

	writeJSON(w, http.StatusOK, riskResponse{
		Table:       table,
		Stats:       stats,
		OverallRisk: overallRisk,
		Columns:     colRisks,
		Advice:      advice,
	})
}

func max2(a, b string) string {
	rank := map[string]int{"LOW": 0, "MEDIUM": 1, "HIGH": 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
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

	writeJSON(w, http.StatusOK, toDiffResponse(schema.Diff(fromERD, toERD, from, to)))
}

// toDiffResponse reshapes the internal SchemaDiff into the format the dashboard expects.
func toDiffResponse(d *schema.SchemaDiff) any {
	type addedCol struct {
		Table  string `json:"table"`
		Column string `json:"column"`
		Type   string `json:"type"`
	}
	type removedCol struct {
		Table  string `json:"table"`
		Column string `json:"column"`
	}
	type modifiedCol struct {
		Table   string `json:"table"`
		Column  string `json:"column"`
		OldType string `json:"oldType"`
		NewType string `json:"newType"`
	}

	added := struct {
		Tables  []string   `json:"tables"`
		Columns []addedCol `json:"columns"`
	}{Tables: d.Added, Columns: []addedCol{}}

	removed := struct {
		Tables  []string     `json:"tables"`
		Columns []removedCol `json:"columns"`
	}{Tables: d.Removed, Columns: []removedCol{}}

	modified := struct {
		Columns []modifiedCol `json:"columns"`
	}{Columns: []modifiedCol{}}

	if added.Tables == nil {
		added.Tables = []string{}
	}
	if removed.Tables == nil {
		removed.Tables = []string{}
	}

	for _, td := range d.Modified {
		for _, c := range td.AddedColumns {
			added.Columns = append(added.Columns, addedCol{Table: td.Table, Column: c.Name, Type: c.Type})
		}
		for _, c := range td.RemovedColumns {
			removed.Columns = append(removed.Columns, removedCol{Table: td.Table, Column: c.Name})
		}
		for _, c := range td.ModifiedColumns {
			modified.Columns = append(modified.Columns, modifiedCol{
				Table:   td.Table,
				Column:  c.Name,
				OldType: c.OldType,
				NewType: c.NewType,
			})
		}
	}

	return map[string]any{
		"from":     d.From,
		"to":       d.To,
		"added":    added,
		"removed":  removed,
		"modified": modified,
	}
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
