package schema_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kubixhq/kubix-schema/internal/schema"
)

// ── Happy path ──────────────────────────────────────────────────────────────

func TestExtractERD_Empty(t *testing.T) {
	db := testDB(t)
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'`).Scan(&count)
	if count > 0 {
		t.Skipf("database has %d table(s); needs an empty public schema", count)
	}
	erd, err := schema.ExtractERD(db)
	if err != nil {
		t.Fatalf("ExtractERD: %v", err)
	}
	if len(erd.Tables) != 0 {
		t.Errorf("expected 0 tables, got %d", len(erd.Tables))
	}
}

func TestExtractERD_SingleTable(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	name := p + "users"
	mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT)`, name))
	dropTable(t, db, name)

	erd, err := schema.ExtractERD(db)
	if err != nil {
		t.Fatalf("ExtractERD: %v", err)
	}
	tbl := findTable(erd, name)
	if tbl == nil {
		t.Fatalf("table %s not found in ERD", name)
	}
	if len(tbl.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(tbl.Columns))
	}
}

// ── Data types ───────────────────────────────────────────────────────────────

func TestExtractERD_AllDataTypes(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	name := p + "types"
	mustExec(t, db, fmt.Sprintf(`
		CREATE TABLE %s (
			col_int     INTEGER,
			col_varchar VARCHAR(255),
			col_text    TEXT,
			col_jsonb   JSONB,
			col_uuid    UUID,
			col_ts      TIMESTAMP,
			col_bool    BOOLEAN,
			col_arr     TEXT[]
		)`, name))
	dropTable(t, db, name)

	erd, err := schema.ExtractERD(db)
	if err != nil {
		t.Fatalf("ExtractERD: %v", err)
	}
	tbl := findTable(erd, name)
	if tbl == nil {
		t.Fatalf("table %s not found", name)
	}
	if len(tbl.Columns) != 8 {
		t.Errorf("expected 8 columns, got %d", len(tbl.Columns))
	}
	want := map[string]string{
		"col_int":   "integer",
		"col_text":  "text",
		"col_jsonb": "jsonb",
		"col_uuid":  "uuid",
		"col_bool":  "boolean",
	}
	for _, col := range tbl.Columns {
		if expected, ok := want[col.Name]; ok && col.Type != expected {
			t.Errorf("col %s: type %q, want %q", col.Name, col.Type, expected)
		}
	}
}

// ── Primary keys ─────────────────────────────────────────────────────────────

func TestExtractERD_NoPrimaryKey(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	name := p + "nopk"
	mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s (x INT, y INT)`, name))
	dropTable(t, db, name)

	erd, _ := schema.ExtractERD(db)
	tbl := findTable(erd, name)
	if tbl == nil {
		t.Fatalf("table %s not found", name)
	}
	if len(tbl.PrimaryKeys) != 0 {
		t.Errorf("expected no PKs, got %v", tbl.PrimaryKeys)
	}
}

func TestExtractERD_CompositePrimaryKey(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	name := p + "comp"
	mustExec(t, db, fmt.Sprintf(`
		CREATE TABLE %s (
			tenant_id INT NOT NULL,
			user_id   INT NOT NULL,
			PRIMARY KEY (tenant_id, user_id)
		)`, name))
	dropTable(t, db, name)

	erd, _ := schema.ExtractERD(db)
	tbl := findTable(erd, name)
	if tbl == nil {
		t.Fatalf("table %s not found", name)
	}
	if len(tbl.PrimaryKeys) != 2 {
		t.Errorf("expected 2 PKs, got %v", tbl.PrimaryKeys)
	}
	pkSet := make(map[string]bool)
	for _, pk := range tbl.PrimaryKeys {
		pkSet[pk] = true
	}
	if !pkSet["tenant_id"] || !pkSet["user_id"] {
		t.Errorf("unexpected PKs: %v", tbl.PrimaryKeys)
	}
}

// ── Foreign keys ─────────────────────────────────────────────────────────────

func TestExtractERD_SelfReferencingFK(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	name := p + "cats"
	mustExec(t, db, fmt.Sprintf(`
		CREATE TABLE %s (
			id        SERIAL PRIMARY KEY,
			parent_id INT REFERENCES %s(id)
		)`, name, name))
	dropTable(t, db, name)

	erd, _ := schema.ExtractERD(db)
	tbl := findTable(erd, name)
	if tbl == nil {
		t.Fatalf("table %s not found", name)
	}
	if len(tbl.ForeignKeys) != 1 {
		t.Fatalf("expected 1 FK, got %d", len(tbl.ForeignKeys))
	}
	fk := tbl.ForeignKeys[0]
	if fk.Column != "parent_id" || fk.ReferencedTable != name || fk.ReferencedColumn != "id" {
		t.Errorf("unexpected self-ref FK: %+v", fk)
	}
}

func TestExtractERD_CircularFK(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	a, b := p+"a", p+"b"

	mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s (id SERIAL PRIMARY KEY, b_id INT)`, a))
	mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s (id SERIAL PRIMARY KEY, a_id INT)`, b))
	mustExec(t, db, fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s_fkb FOREIGN KEY (b_id) REFERENCES %s(id) DEFERRABLE`, a, a, b))
	mustExec(t, db, fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s_fka FOREIGN KEY (a_id) REFERENCES %s(id) DEFERRABLE`, b, b, a))
	dropTable(t, db, a)
	dropTable(t, db, b)

	erd, err := schema.ExtractERD(db)
	if err != nil {
		t.Fatalf("ExtractERD must not fail on circular FKs: %v", err)
	}
	tblA, tblB := findTable(erd, a), findTable(erd, b)
	if tblA == nil || tblB == nil {
		t.Fatal("both tables must be present")
	}
	if len(tblA.ForeignKeys) != 1 || tblA.ForeignKeys[0].ReferencedTable != b {
		t.Errorf("%s FK: want ref to %s, got %+v", a, b, tblA.ForeignKeys)
	}
	if len(tblB.ForeignKeys) != 1 || tblB.ForeignKeys[0].ReferencedTable != a {
		t.Errorf("%s FK: want ref to %s, got %+v", b, a, tblB.ForeignKeys)
	}
}

// ── Schema / views ───────────────────────────────────────────────────────────

func TestExtractERD_ViewsExcluded(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	base, vw := p+"base", p+"vw"
	mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s (id SERIAL PRIMARY KEY, val TEXT)`, base))
	mustExec(t, db, fmt.Sprintf(`CREATE VIEW %s AS SELECT id, val FROM %s`, vw, base))
	dropTable(t, db, base)
	dropView(t, db, vw)

	erd, _ := schema.ExtractERD(db)
	if findTable(erd, vw) != nil {
		t.Errorf("view %s must not appear in ERD tables", vw)
	}
	if findTable(erd, base) == nil {
		t.Errorf("base table %s must be in ERD", base)
	}
}

func TestExtractERD_CustomSchemaExcluded(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	schName := "sch_" + p[:min(len(p), 8)]
	tblName := p + "mytbl"

	mustExec(t, db, fmt.Sprintf(`CREATE SCHEMA %s`, schName))
	mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s.%s (id INT)`, schName, tblName))
	dropSchema(t, db, schName)

	erd, _ := schema.ExtractERD(db)
	// ERD only queries the public schema — this table must not appear
	if findTable(erd, tblName) != nil {
		t.Errorf("table from custom schema %s must not appear in ERD", schName)
	}
}

func TestExtractERD_SameNameDifferentSchemas(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	schName := "sch2_" + p[:min(len(p), 7)]
	publicTable := p + "shared"

	mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s (id SERIAL PRIMARY KEY)`, publicTable))
	mustExec(t, db, fmt.Sprintf(`CREATE SCHEMA %s`, schName))
	mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s.%s (id INT)`, schName, publicTable))
	dropTable(t, db, publicTable)
	dropSchema(t, db, schName)

	erd, _ := schema.ExtractERD(db)
	count := 0
	for _, tbl := range erd.Tables {
		if tbl.Name == publicTable {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 table named %s, got %d", publicTable, count)
	}
}

// ── Columns ──────────────────────────────────────────────────────────────────

func TestExtractERD_NullableColumns(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	name := p + "null"
	mustExec(t, db, fmt.Sprintf(`
		CREATE TABLE %s (required TEXT NOT NULL, optional TEXT)`, name))
	dropTable(t, db, name)

	erd, _ := schema.ExtractERD(db)
	tbl := findTable(erd, name)
	if tbl == nil {
		t.Fatalf("table %s not found", name)
	}
	if colByName(tbl, "required").Nullable {
		t.Error("required should be NOT NULL (nullable=false)")
	}
	if !colByName(tbl, "optional").Nullable {
		t.Error("optional should be nullable (nullable=true)")
	}
}

func TestExtractERD_DefaultValues(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	name := p + "defs"
	mustExec(t, db, fmt.Sprintf(`
		CREATE TABLE %s (
			id         SERIAL,
			created_at TIMESTAMP DEFAULT now(),
			status     TEXT DEFAULT 'active',
			no_default TEXT
		)`, name))
	dropTable(t, db, name)

	erd, _ := schema.ExtractERD(db)
	tbl := findTable(erd, name)
	if tbl == nil {
		t.Fatalf("table %s not found", name)
	}
	if colByName(tbl, "created_at").Default == nil {
		t.Error("created_at must have a default")
	}
	if colByName(tbl, "status").Default == nil {
		t.Error("status must have a default")
	}
	if colByName(tbl, "no_default").Default != nil {
		t.Errorf("no_default must have nil default, got %q", *colByName(tbl, "no_default").Default)
	}
}

func TestExtractERD_GeneratedColumns(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	name := p + "gen"
	_, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			price    NUMERIC(10,2),
			tax_rate NUMERIC(5,4),
			total    NUMERIC(10,2) GENERATED ALWAYS AS (price * (1 + tax_rate)) STORED
		)`, name))
	if err != nil {
		t.Skipf("generated columns not supported by this Postgres version: %v", err)
	}
	dropTable(t, db, name)

	erd, _ := schema.ExtractERD(db)
	tbl := findTable(erd, name)
	if tbl == nil {
		t.Fatalf("table %s not found", name)
	}
	if colByName(tbl, "total") == nil {
		t.Error("generated column 'total' must appear in the column list")
	}
}

// ── Scale ────────────────────────────────────────────────────────────────────

func TestExtractERD_LargeSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large-schema test in -short mode")
	}
	db := testDB(t)
	p := testPrefix(t)

	const n = 101
	for i := range n {
		name := fmt.Sprintf("%stbl%03d", p, i)
		mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s (id SERIAL PRIMARY KEY)`, name))
		dropTable(t, db, name)
	}

	start := time.Now()
	erd, err := schema.ExtractERD(db)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ExtractERD: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("ExtractERD took %v; must complete in <5s", elapsed)
	}

	found := 0
	for i := range n {
		if findTable(erd, fmt.Sprintf("%stbl%03d", p, i)) != nil {
			found++
		}
	}
	if found != n {
		t.Errorf("expected all %d tables in ERD, found %d", n, found)
	}
}

// ── Concurrency ──────────────────────────────────────────────────────────────

func TestExtractERD_ConcurrentRequests(t *testing.T) {
	db := testDB(t)
	p := testPrefix(t)
	name := p + "race"
	mustExec(t, db, fmt.Sprintf(`CREATE TABLE %s (id SERIAL PRIMARY KEY)`, name))
	dropTable(t, db, name)

	errs := make(chan error, 10)
	for range 10 {
		go func() {
			_, err := schema.ExtractERD(db)
			errs <- err
		}()
	}
	for range 10 {
		if err := <-errs; err != nil {
			t.Errorf("concurrent ExtractERD error: %v", err)
		}
	}
}
