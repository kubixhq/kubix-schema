package schema_test

// diff_test.go contains pure unit tests — no database required.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kubixhq/kubix-schema/internal/schema"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func strPtr(s string) *string { return &s }

func makeERD(tables ...schema.Table) *schema.ERD {
	return &schema.ERD{Tables: tables}
}

func makeTable(name string, cols []schema.Column, pks []string, fks []schema.ForeignKey) schema.Table {
	if cols == nil {
		cols = []schema.Column{}
	}
	if pks == nil {
		pks = []string{}
	}
	if fks == nil {
		fks = []schema.ForeignKey{}
	}
	return schema.Table{Name: name, Columns: cols, PrimaryKeys: pks, ForeignKeys: fks}
}

func col(name, typ string, nullable bool) schema.Column {
	return schema.Column{Name: name, Type: typ, Nullable: nullable}
}

func colDefault(name, typ, def string) schema.Column {
	return schema.Column{Name: name, Type: typ, Nullable: true, Default: strPtr(def)}
}

// ── No change ────────────────────────────────────────────────────────────────

func TestDiff_Identical(t *testing.T) {
	erd := makeERD(makeTable("users", []schema.Column{col("id", "integer", false)}, []string{"id"}, nil))
	d := schema.Diff(erd, erd, "v1", "v1")
	if len(d.Added)+len(d.Removed)+len(d.Modified) != 0 {
		t.Errorf("identical ERDs should produce empty diff: %+v", d)
	}
}

func TestDiff_FromEqualsTo(t *testing.T) {
	erd := makeERD(makeTable("orders", []schema.Column{col("id", "integer", false)}, []string{"id"}, nil))
	d := schema.Diff(erd, erd, "v2", "v2")
	if len(d.Added) != 0 || len(d.Removed) != 0 || len(d.Modified) != 0 {
		t.Errorf("from==to should produce empty diff, got %+v", d)
	}
}

// ── Tables added / removed ───────────────────────────────────────────────────

func TestDiff_TableAdded(t *testing.T) {
	from := makeERD(makeTable("users", nil, nil, nil))
	to := makeERD(
		makeTable("users", nil, nil, nil),
		makeTable("posts", nil, nil, nil),
	)
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Added) != 1 || d.Added[0] != "posts" {
		t.Errorf("expected [posts] added, got %v", d.Added)
	}
	if len(d.Removed) != 0 {
		t.Errorf("expected no removed tables, got %v", d.Removed)
	}
}

func TestDiff_TableRemoved(t *testing.T) {
	from := makeERD(
		makeTable("users", nil, nil, nil),
		makeTable("sessions", nil, nil, nil),
	)
	to := makeERD(makeTable("users", nil, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Removed) != 1 || d.Removed[0] != "sessions" {
		t.Errorf("expected [sessions] removed, got %v", d.Removed)
	}
}

// Table rename shows as removed + added (cannot distinguish from snapshots alone).
func TestDiff_TableRenamed_ShowsAsRemovedAndAdded(t *testing.T) {
	from := makeERD(makeTable("old_name", nil, nil, nil))
	to := makeERD(makeTable("new_name", nil, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Removed) != 1 || d.Removed[0] != "old_name" {
		t.Errorf("expected old_name removed, got %v", d.Removed)
	}
	if len(d.Added) != 1 || d.Added[0] != "new_name" {
		t.Errorf("expected new_name added, got %v", d.Added)
	}
}

// ── Columns ──────────────────────────────────────────────────────────────────

func TestDiff_ColumnAdded(t *testing.T) {
	from := makeERD(makeTable("users", []schema.Column{col("id", "integer", false)}, nil, nil))
	to := makeERD(makeTable("users", []schema.Column{
		col("id", "integer", false),
		col("email", "text", false),
	}, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified) != 1 {
		t.Fatalf("expected 1 modified table, got %d", len(d.Modified))
	}
	if len(d.Modified[0].AddedColumns) != 1 || d.Modified[0].AddedColumns[0].Name != "email" {
		t.Errorf("expected email added, got %+v", d.Modified[0].AddedColumns)
	}
}

func TestDiff_ColumnRemoved(t *testing.T) {
	from := makeERD(makeTable("users", []schema.Column{
		col("id", "integer", false),
		col("bio", "text", true),
	}, nil, nil))
	to := makeERD(makeTable("users", []schema.Column{col("id", "integer", false)}, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified[0].RemovedColumns) != 1 || d.Modified[0].RemovedColumns[0].Name != "bio" {
		t.Errorf("expected bio removed, got %+v", d.Modified[0].RemovedColumns)
	}
}

// Column rename shows as removed + added.
func TestDiff_ColumnRenamed_ShowsAsRemovedAndAdded(t *testing.T) {
	from := makeERD(makeTable("users", []schema.Column{col("old_col", "text", true)}, nil, nil))
	to := makeERD(makeTable("users", []schema.Column{col("new_col", "text", true)}, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	td := d.Modified[0]
	if len(td.RemovedColumns) != 1 || td.RemovedColumns[0].Name != "old_col" {
		t.Errorf("expected old_col removed, got %+v", td.RemovedColumns)
	}
	if len(td.AddedColumns) != 1 || td.AddedColumns[0].Name != "new_col" {
		t.Errorf("expected new_col added, got %+v", td.AddedColumns)
	}
}

// ── Column modifications ─────────────────────────────────────────────────────

func TestDiff_ColumnTypeChanged(t *testing.T) {
	from := makeERD(makeTable("users", []schema.Column{col("code", "character varying", false)}, nil, nil))
	to := makeERD(makeTable("users", []schema.Column{col("code", "text", false)}, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified[0].ModifiedColumns) != 1 {
		t.Fatalf("expected 1 modified column, got %d", len(d.Modified[0].ModifiedColumns))
	}
	cc := d.Modified[0].ModifiedColumns[0]
	if cc.OldType != "character varying" || cc.NewType != "text" {
		t.Errorf("wrong type change: %+v", cc)
	}
}

func TestDiff_ColumnNullableChanged(t *testing.T) {
	from := makeERD(makeTable("users", []schema.Column{col("email", "text", true)}, nil, nil))
	to := makeERD(makeTable("users", []schema.Column{col("email", "text", false)}, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified[0].ModifiedColumns) != 1 {
		t.Fatalf("expected 1 modified column, got %d", len(d.Modified[0].ModifiedColumns))
	}
	cc := d.Modified[0].ModifiedColumns[0]
	if cc.OldNullable == nil || *cc.OldNullable != true {
		t.Errorf("OldNullable should be true, got %v", cc.OldNullable)
	}
	if cc.NewNullable == nil || *cc.NewNullable != false {
		t.Errorf("NewNullable should be false, got %v", cc.NewNullable)
	}
}

func TestDiff_ColumnDefaultAdded(t *testing.T) {
	from := makeERD(makeTable("users", []schema.Column{col("status", "text", true)}, nil, nil))
	to := makeERD(makeTable("users", []schema.Column{colDefault("status", "text", "'active'")}, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified[0].ModifiedColumns) != 1 {
		t.Fatalf("expected 1 modified column, got %d", len(d.Modified[0].ModifiedColumns))
	}
	cc := d.Modified[0].ModifiedColumns[0]
	if !cc.DefaultChanged {
		t.Error("DefaultChanged must be true")
	}
	if cc.OldDefault != nil {
		t.Errorf("OldDefault must be nil (no default), got %v", cc.OldDefault)
	}
	if cc.NewDefault == nil || *cc.NewDefault != "'active'" {
		t.Errorf("NewDefault should be \"'active'\", got %v", cc.NewDefault)
	}
}

func TestDiff_ColumnDefaultRemoved(t *testing.T) {
	from := makeERD(makeTable("users", []schema.Column{colDefault("status", "text", "'active'")}, nil, nil))
	to := makeERD(makeTable("users", []schema.Column{col("status", "text", true)}, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	cc := d.Modified[0].ModifiedColumns[0]
	if !cc.DefaultChanged {
		t.Error("DefaultChanged must be true")
	}
	if cc.OldDefault == nil || *cc.OldDefault != "'active'" {
		t.Errorf("OldDefault should be \"'active'\", got %v", cc.OldDefault)
	}
	if cc.NewDefault != nil {
		t.Errorf("NewDefault must be nil (default removed), got %v", cc.NewDefault)
	}
}

func TestDiff_ColumnDefaultChanged(t *testing.T) {
	from := makeERD(makeTable("t", []schema.Column{colDefault("c", "text", "''")}, nil, nil))
	to := makeERD(makeTable("t", []schema.Column{colDefault("c", "text", "'n/a'")}, nil, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified[0].ModifiedColumns) != 1 {
		t.Fatalf("expected 1 modified column, got %d", len(d.Modified[0].ModifiedColumns))
	}
	cc := d.Modified[0].ModifiedColumns[0]
	if !cc.DefaultChanged {
		t.Error("DefaultChanged must be true")
	}
}

// ── Constraints / FKs ────────────────────────────────────────────────────────

func TestDiff_PKAdded(t *testing.T) {
	from := makeERD(makeTable("t", []schema.Column{col("id", "integer", false)}, []string{}, nil))
	to := makeERD(makeTable("t", []schema.Column{col("id", "integer", false)}, []string{"id"}, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified[0].AddedConstraints) != 1 {
		t.Fatalf("expected 1 added constraint, got %v", d.Modified[0].AddedConstraints)
	}
}

func TestDiff_PKRemoved(t *testing.T) {
	from := makeERD(makeTable("t", []schema.Column{col("id", "integer", false)}, []string{"id"}, nil))
	to := makeERD(makeTable("t", []schema.Column{col("id", "integer", false)}, []string{}, nil))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified[0].RemovedConstraints) != 1 {
		t.Fatalf("expected 1 removed constraint, got %v", d.Modified[0].RemovedConstraints)
	}
}

func TestDiff_FKAdded(t *testing.T) {
	fk := schema.ForeignKey{Column: "user_id", ReferencedTable: "users", ReferencedColumn: "id"}
	from := makeERD(makeTable("posts", []schema.Column{col("user_id", "integer", false)}, nil, []schema.ForeignKey{}))
	to := makeERD(makeTable("posts", []schema.Column{col("user_id", "integer", false)}, nil, []schema.ForeignKey{fk}))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified[0].AddedForeignKeys) != 1 {
		t.Fatalf("expected 1 added FK, got %v", d.Modified[0].AddedForeignKeys)
	}
	if d.Modified[0].AddedForeignKeys[0].ReferencedTable != "users" {
		t.Errorf("wrong FK target: %+v", d.Modified[0].AddedForeignKeys[0])
	}
}

func TestDiff_FKRemoved(t *testing.T) {
	fk := schema.ForeignKey{Column: "user_id", ReferencedTable: "users", ReferencedColumn: "id"}
	from := makeERD(makeTable("posts", nil, nil, []schema.ForeignKey{fk}))
	to := makeERD(makeTable("posts", nil, nil, []schema.ForeignKey{}))
	d := schema.Diff(from, to, "v1", "v2")
	if len(d.Modified[0].RemovedForeignKeys) != 1 {
		t.Fatalf("expected 1 removed FK, got %v", d.Modified[0].RemovedForeignKeys)
	}
}

// ── Reverse diff ─────────────────────────────────────────────────────────────

func TestDiff_Backward(t *testing.T) {
	v1 := makeERD(makeTable("users", []schema.Column{col("id", "integer", false)}, nil, nil))
	v2 := makeERD(makeTable("users", []schema.Column{
		col("id", "integer", false),
		col("email", "text", false),
	}, nil, nil))
	// Forward: email added
	fwd := schema.Diff(v1, v2, "v1", "v2")
	if len(fwd.Modified[0].AddedColumns) != 1 {
		t.Errorf("forward: expected 1 added col, got %d", len(fwd.Modified[0].AddedColumns))
	}
	// Backward: email removed
	bwd := schema.Diff(v2, v1, "v2", "v1")
	if len(bwd.Modified[0].RemovedColumns) != 1 {
		t.Errorf("backward: expected 1 removed col, got %d", len(bwd.Modified[0].RemovedColumns))
	}
}

// ── Snapshots ────────────────────────────────────────────────────────────────

func TestSaveAndLoadSnapshot_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	erd := makeERD(makeTable("users", []schema.Column{col("id", "integer", false)}, []string{"id"}, nil))

	if err := schema.SaveSnapshot(erd, "v1", dir); err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	loaded, err := schema.LoadSnapshot(nil, "v1", dir)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if len(loaded.Tables) != 1 || loaded.Tables[0].Name != "users" {
		t.Errorf("round-trip mismatch: %+v", loaded)
	}
}

func TestLoadSnapshot_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := schema.LoadSnapshot(nil, "missing_version", dir)
	if err == nil {
		t.Fatal("expected error for missing snapshot")
	}
	if !contains(err.Error(), "snapshot not found") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestLoadSnapshot_CorruptedJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not valid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := schema.LoadSnapshot(nil, "bad", dir)
	if err == nil {
		t.Fatal("expected error for corrupt snapshot")
	}
	if !contains(err.Error(), "corrupt snapshot") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestLoadSnapshot_SnapshotDirAutoCreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent", "snapshots")
	// dir does not exist yet

	// First save creates the dir
	erd := makeERD(makeTable("t", nil, nil, nil))
	if err := schema.SaveSnapshot(erd, "v1", dir); err != nil {
		t.Fatalf("SaveSnapshot should create dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("snapshot dir should exist after SaveSnapshot: %v", err)
	}

	// Load from a dir that exists but has no file for version "v2"
	_, err := schema.LoadSnapshot(nil, "v2", dir)
	if err == nil || !contains(err.Error(), "snapshot not found") {
		t.Errorf("expected 'snapshot not found', got %v", err)
	}
}

func TestSaveSnapshot_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	erd := makeERD(
		makeTable("users", []schema.Column{col("id", "integer", false), col("name", "text", true)}, []string{"id"}, nil),
	)
	schema.SaveSnapshot(erd, "v2", dir)

	data, _ := os.ReadFile(filepath.Join(dir, "v2.json"))
	var parsed schema.ERD
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("saved snapshot is not valid JSON: %v", err)
	}
}

// ── Helper ───────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
