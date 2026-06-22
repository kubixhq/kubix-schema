package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/kubixhq/kubix-schema/internal/config"
	"github.com/kubixhq/kubix-schema/internal/handler"
	"github.com/kubixhq/kubix-schema/internal/schema"
)

// ── Test setup ───────────────────────────────────────────────────────────────

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN not set — skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// closedDB returns a *sql.DB that is already closed — simulates DB unavailability.
func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("postgres", "host=localhost user=nobody dbname=nobody")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	return db
}

func newMux(db *sql.DB, snapshotDir string) *http.ServeMux {
	cfg := config.Config{MigrationTool: "auto", SnapshotDir: snapshotDir}
	h := handler.New(db, cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/schema/erd", h.ERD)
	mux.HandleFunc("GET /api/schema/migrations", h.Migrations)
	mux.HandleFunc("GET /api/schema/diff", h.Diff)
	return mux
}

func do(mux *http.ServeMux, method, path string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(method, path, nil))
	return rr
}

func errMsg(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Error string }
	json.NewDecoder(rr.Body).Decode(&body)
	return body.Error
}

// ── HTTP method / routing ────────────────────────────────────────────────────

func TestHandler_ERD_MethodNotAllowed(t *testing.T) {
	mux := newMux(closedDB(t), t.TempDir())
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		rr := do(mux, method, "/api/schema/erd")
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/schema/erd: got %d, want 405", method, rr.Code)
		}
	}
}

func TestHandler_Migrations_MethodNotAllowed(t *testing.T) {
	mux := newMux(closedDB(t), t.TempDir())
	rr := do(mux, "POST", "/api/schema/migrations")
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/schema/migrations: got %d, want 405", rr.Code)
	}
}

func TestHandler_Diff_MethodNotAllowed(t *testing.T) {
	mux := newMux(closedDB(t), t.TempDir())
	rr := do(mux, "POST", "/api/schema/diff")
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/schema/diff: got %d, want 405", rr.Code)
	}
}

func TestHandler_WrongEndpoint_404(t *testing.T) {
	mux := newMux(closedDB(t), t.TempDir())
	for _, path := range []string{"/", "/api/schema", "/api/schema/unknown", "/api"} {
		rr := do(mux, "GET", path)
		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404", path, rr.Code)
		}
	}
}

// ── Diff query param validation ──────────────────────────────────────────────

func TestHandler_Diff_MissingFrom(t *testing.T) {
	mux := newMux(closedDB(t), t.TempDir())
	rr := do(mux, "GET", "/api/schema/diff?to=v2")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing from: got %d, want 400", rr.Code)
	}
	if !strings.Contains(errMsg(t, rr), "from") {
		t.Errorf("error message should mention 'from': %s", rr.Body.String())
	}
}

func TestHandler_Diff_MissingTo(t *testing.T) {
	mux := newMux(closedDB(t), t.TempDir())
	rr := do(mux, "GET", "/api/schema/diff?from=v1")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing to: got %d, want 400", rr.Code)
	}
	if !strings.Contains(errMsg(t, rr), "to") {
		t.Errorf("error message should mention 'to': %s", rr.Body.String())
	}
}

func TestHandler_Diff_BothMissing(t *testing.T) {
	mux := newMux(closedDB(t), t.TempDir())
	rr := do(mux, "GET", "/api/schema/diff")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("both missing: got %d, want 400", rr.Code)
	}
}

// ── Snapshot errors ──────────────────────────────────────────────────────────

func TestHandler_Diff_SnapshotNotFound(t *testing.T) {
	dir := t.TempDir() // empty — no snapshots stored
	mux := newMux(closedDB(t), dir)
	rr := do(mux, "GET", "/api/schema/diff?from=nonexistent&to=also_missing")
	if rr.Code != http.StatusNotFound {
		t.Errorf("missing snapshot: got %d, want 404", rr.Code)
	}
}

func TestHandler_Diff_CorruptedSnapshot_Returns500(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not valid json"), 0644)

	mux := newMux(closedDB(t), dir)
	rr := do(mux, "GET", "/api/schema/diff?from=bad&to=bad")
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("corrupt snapshot: got %d, want 500", rr.Code)
	}
	if !strings.Contains(errMsg(t, rr), "corrupt") {
		t.Errorf("error should mention corruption: %s", rr.Body.String())
	}
}

func TestHandler_Diff_SnapshotDirMissing_AutoCreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does_not_exist_yet")
	mux := newMux(closedDB(t), dir)
	rr := do(mux, "GET", "/api/schema/diff?from=v1&to=v2")
	// Dir didn't exist — should be created, then return 404 for missing snapshot
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing snapshot after auto-creating dir, got %d", rr.Code)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("snapshot dir should have been created: %v", err)
	}
}

// ── DB errors → 503 ──────────────────────────────────────────────────────────

func TestHandler_ERD_DBUnavailable_Returns503(t *testing.T) {
	mux := newMux(closedDB(t), t.TempDir())
	rr := do(mux, "GET", "/api/schema/erd")
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("closed DB: got %d, want 503", rr.Code)
	}
}

func TestHandler_Migrations_DBUnavailable_Returns503(t *testing.T) {
	mux := newMux(closedDB(t), t.TempDir())
	rr := do(mux, "GET", "/api/schema/migrations")
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("closed DB: got %d, want 503", rr.Code)
	}
}

// ── Response format ───────────────────────────────────────────────────────────

func TestHandler_ERD_ContentType(t *testing.T) {
	db := testDB(t)
	mux := newMux(db, t.TempDir())
	rr := do(mux, "GET", "/api/schema/erd")
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: %q, want application/json", ct)
	}
}

func TestHandler_Migrations_ReturnsArray(t *testing.T) {
	db := testDB(t)
	mux := newMux(db, t.TempDir())
	rr := do(mux, "GET", "/api/schema/migrations")
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	var result []json.RawMessage
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("migrations response must be a JSON array: %v", err)
	}
}

func TestHandler_ERD_ReturnsTableArray(t *testing.T) {
	db := testDB(t)
	mux := newMux(db, t.TempDir())
	rr := do(mux, "GET", "/api/schema/erd")
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	var result schema.ERD
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("ERD response invalid: %v", err)
	}
	if result.Tables == nil {
		t.Error("tables field must not be nil")
	}
}

// ── Diff integration ──────────────────────────────────────────────────────────

// TestHandler_Diff_IdenticalSnapshots diffs two snapshots that were captured
// from the same ERD object — result must always be empty regardless of other
// tests modifying the DB concurrently.
func TestHandler_Diff_IdenticalSnapshots(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	erd, err := schema.ExtractERD(db)
	if err != nil {
		t.Fatalf("ExtractERD: %v", err)
	}
	schema.SaveSnapshot(erd, "v1", dir)
	schema.SaveSnapshot(erd, "v2", dir) // identical copy

	mux := newMux(db, dir)
	rr := do(mux, "GET", "/api/schema/diff?from=v1&to=v2")
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rr.Code)
	}
	var d schema.SchemaDiff
	json.NewDecoder(rr.Body).Decode(&d)
	if len(d.Added)+len(d.Removed)+len(d.Modified) != 0 {
		t.Errorf("identical snapshots should produce empty diff: %+v", d)
	}
}

// TestHandler_Diff_CurrentKeyword verifies that "current" is accepted as a
// version key and returns a valid JSON diff (content varies by DB state).
func TestHandler_Diff_CurrentKeyword(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	erd, err := schema.ExtractERD(db)
	if err != nil {
		t.Fatalf("ExtractERD: %v", err)
	}
	schema.SaveSnapshot(erd, "baseline", dir)

	mux := newMux(db, dir)
	rr := do(mux, "GET", "/api/schema/diff?from=baseline&to=current")
	if rr.Code != http.StatusOK {
		t.Fatalf("from=baseline&to=current: got %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var d schema.SchemaDiff
	if err := json.NewDecoder(rr.Body).Decode(&d); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	// Structural validity — arrays must be non-nil
	if d.Added == nil || d.Removed == nil || d.Modified == nil {
		t.Errorf("diff fields must not be nil: %+v", d)
	}
}

// ── Config validation ─────────────────────────────────────────────────────────

func TestConfig_Validate_ValidTools(t *testing.T) {
	for _, tool := range []string{"auto", "flyway", "liquibase", "prisma"} {
		cfg := config.Config{DBPort: 5432, MigrationTool: tool}
		if err := cfg.Validate(); err != nil {
			t.Errorf("tool %q: unexpected error: %v", tool, err)
		}
	}
}

func TestConfig_Validate_InvalidTool(t *testing.T) {
	cfg := config.Config{DBPort: 5432, MigrationTool: "oracle"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for unknown MIGRATION_TOOL")
	}
}

func TestConfig_Validate_InvalidPort_Zero(t *testing.T) {
	cfg := config.Config{DBPort: 0, MigrationTool: "auto"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for port 0 (non-numeric DB_PORT)")
	}
}

func TestConfig_Validate_InvalidPort_OutOfRange(t *testing.T) {
	cfg := config.Config{DBPort: 99999, MigrationTool: "auto"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for port > 65535")
	}
}
