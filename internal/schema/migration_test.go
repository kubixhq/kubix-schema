package schema_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kubixhq/kubix-schema/internal/schema"
)

// flywayDDL is the minimal Flyway schema history table.
const flywayDDL = `CREATE TABLE flyway_schema_history (
	installed_rank INT         NOT NULL PRIMARY KEY,
	version        VARCHAR(50),
	description    VARCHAR(200) NOT NULL,
	type           VARCHAR(20)  NOT NULL,
	script         VARCHAR(1000) NOT NULL,
	checksum       INT,
	installed_by   VARCHAR(100) NOT NULL,
	installed_on   TIMESTAMP    NOT NULL DEFAULT now(),
	execution_time INT          NOT NULL,
	success        BOOLEAN      NOT NULL
)`

const liquibaseDDL = `CREATE TABLE databasechangelog (
	id              VARCHAR(255) NOT NULL,
	author          VARCHAR(255) NOT NULL,
	filename        VARCHAR(255) NOT NULL,
	dateexecuted    TIMESTAMP    NOT NULL,
	orderexecuted   INT          NOT NULL,
	exectype        VARCHAR(10)  NOT NULL,
	md5sum          VARCHAR(35),
	description     VARCHAR(255),
	comments        VARCHAR(255),
	tag             VARCHAR(255),
	liquibase        VARCHAR(20),
	contexts        VARCHAR(255),
	labels          VARCHAR(255),
	deployment_id   VARCHAR(10)
)`

const prismaDDL = `CREATE TABLE _prisma_migrations (
	id                    VARCHAR(36)  NOT NULL PRIMARY KEY,
	checksum              VARCHAR(64)  NOT NULL,
	finished_at           TIMESTAMP,
	migration_name        VARCHAR(255) NOT NULL,
	logs                  TEXT,
	rolled_back_at        TIMESTAMP,
	started_at            TIMESTAMP    NOT NULL DEFAULT now(),
	applied_steps_count   INT          NOT NULL DEFAULT 0
)`

// ── Auto-detection ───────────────────────────────────────────────────────────

func TestDetectAllMigrationTools_Flyway(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") {
		t.Skip("flyway_schema_history already exists")
	}
	mustExec(t, db, flywayDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS flyway_schema_history") })

	tools := schema.DetectAllMigrationTools(db, "auto")
	if len(tools) == 0 || tools[0] != schema.ToolFlyway {
		t.Errorf("expected [flyway], got %v", tools)
	}
}

func TestDetectAllMigrationTools_Liquibase(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "databasechangelog") {
		t.Skip("databasechangelog already exists")
	}
	mustExec(t, db, liquibaseDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS databasechangelog") })

	tools := schema.DetectAllMigrationTools(db, "auto")
	found := false
	for _, tool := range tools {
		if tool == schema.ToolLiquibase {
			found = true
		}
	}
	if !found {
		t.Errorf("liquibase not detected, got %v", tools)
	}
}

func TestDetectAllMigrationTools_Prisma(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "_prisma_migrations") {
		t.Skip("_prisma_migrations already exists")
	}
	mustExec(t, db, prismaDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS _prisma_migrations") })

	tools := schema.DetectAllMigrationTools(db, "auto")
	found := false
	for _, tool := range tools {
		if tool == schema.ToolPrisma {
			found = true
		}
	}
	if !found {
		t.Errorf("prisma not detected, got %v", tools)
	}
}

func TestDetectAllMigrationTools_None(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") || tableExists(db, "databasechangelog") || tableExists(db, "_prisma_migrations") {
		t.Skip("migration tables already exist; needs clean DB")
	}
	tools := schema.DetectAllMigrationTools(db, "auto")
	if len(tools) != 0 {
		t.Errorf("expected no tools, got %v", tools)
	}
}

func TestDetectAllMigrationTools_BothFlywayAndLiquibase(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") || tableExists(db, "databasechangelog") {
		t.Skip("migration tables already exist; needs clean DB")
	}
	mustExec(t, db, flywayDDL)
	mustExec(t, db, liquibaseDDL)
	t.Cleanup(func() {
		_, _ = db.Exec("DROP TABLE IF EXISTS flyway_schema_history")
		_, _ = db.Exec("DROP TABLE IF EXISTS databasechangelog")
	})

	tools := schema.DetectAllMigrationTools(db, "auto")
	if len(tools) < 2 {
		t.Fatalf("expected ≥2 tools, got %v", tools)
	}
	toolSet := make(map[schema.Tool]bool)
	for _, tool := range tools {
		toolSet[tool] = true
	}
	if !toolSet[schema.ToolFlyway] || !toolSet[schema.ToolLiquibase] {
		t.Errorf("expected both flyway and liquibase, got %v", tools)
	}
}

func TestDetectAllMigrationTools_HintOverride(t *testing.T) {
	db := testDB(t)
	// hint bypasses DB probing
	tools := schema.DetectAllMigrationTools(db, "flyway")
	if len(tools) != 1 || tools[0] != schema.ToolFlyway {
		t.Errorf("hint=flyway: expected [flyway], got %v", tools)
	}
}

// ── Flyway ───────────────────────────────────────────────────────────────────

func TestFetchMigrations_Flyway_AllSuccess(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") {
		t.Skip("flyway_schema_history already exists")
	}
	mustExec(t, db, flywayDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS flyway_schema_history") })

	for _, row := range []struct {
		rank int
		ver  string
		desc string
	}{
		{1, "1", "init"},
		{2, "2", "add_users"},
		{3, "3", "add_posts"},
	} {
		mustExec(t, db, fmt.Sprintf(`INSERT INTO flyway_schema_history VALUES (%d,'%s','%s','SQL','V%s.sql',NULL,'test',now(),10,true)`,
			row.rank, row.ver, row.desc, row.ver))
	}

	h, err := schema.FetchMigrations(db, schema.ToolFlyway)
	if err != nil {
		t.Fatalf("FetchMigrations: %v", err)
	}
	if len(h.Migrations) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(h.Migrations))
	}
	for _, m := range h.Migrations {
		if m.Status != "success" {
			t.Errorf("migration %s: want success, got %s", m.Version, m.Status)
		}
	}
}

func TestFetchMigrations_Flyway_WithFailed(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") {
		t.Skip("flyway_schema_history already exists")
	}
	mustExec(t, db, flywayDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS flyway_schema_history") })

	mustExec(t, db, `INSERT INTO flyway_schema_history VALUES (1,'1','init','SQL','V1.sql',NULL,'test',now(),10,true)`)
	mustExec(t, db, `INSERT INTO flyway_schema_history VALUES (2,'2','bad','SQL','V2.sql',NULL,'test',now(),5,false)`)

	h, _ := schema.FetchMigrations(db, schema.ToolFlyway)
	if h.Migrations[1].Status != "failed" {
		t.Errorf("migration 2 should be failed, got %s", h.Migrations[1].Status)
	}
}

func TestFetchMigrations_Flyway_VersionGaps(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") {
		t.Skip("flyway_schema_history already exists")
	}
	mustExec(t, db, flywayDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS flyway_schema_history") })

	mustExec(t, db, `INSERT INTO flyway_schema_history VALUES (1,'1','v1','SQL','V1.sql',NULL,'test',now(),10,true)`)
	mustExec(t, db, `INSERT INTO flyway_schema_history VALUES (2,'2','v2','SQL','V2.sql',NULL,'test',now(),10,true)`)
	mustExec(t, db, `INSERT INTO flyway_schema_history VALUES (5,'5','v5','SQL','V5.sql',NULL,'test',now(),10,true)`)

	h, _ := schema.FetchMigrations(db, schema.ToolFlyway)
	if len(h.Migrations) != 3 {
		t.Errorf("expected 3 migrations (v1, v2, v5), got %d", len(h.Migrations))
	}
	versions := []string{h.Migrations[0].Version, h.Migrations[1].Version, h.Migrations[2].Version}
	if versions[0] != "1" || versions[1] != "2" || versions[2] != "5" {
		t.Errorf("unexpected version order: %v", versions)
	}
}

func TestFetchMigrations_Flyway_Empty(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") {
		t.Skip("flyway_schema_history already exists")
	}
	mustExec(t, db, flywayDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS flyway_schema_history") })

	h, err := schema.FetchMigrations(db, schema.ToolFlyway)
	if err != nil {
		t.Fatalf("FetchMigrations: %v", err)
	}
	if len(h.Migrations) != 0 {
		t.Errorf("expected 0 migrations, got %d", len(h.Migrations))
	}
}

func TestFetchMigrations_Flyway_TableAbsent(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") {
		t.Skip("flyway_schema_history exists; needs clean DB")
	}
	h, err := schema.FetchMigrations(db, schema.ToolFlyway)
	if err != nil {
		t.Fatalf("must not error when table is absent: %v", err)
	}
	if len(h.Migrations) != 0 {
		t.Errorf("expected empty migrations, got %d", len(h.Migrations))
	}
}

func TestFetchMigrations_Flyway_OutOfOrder(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") {
		t.Skip("flyway_schema_history already exists")
	}
	mustExec(t, db, flywayDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS flyway_schema_history") })

	// inserted in reverse order but installed_rank determines final order
	mustExec(t, db, `INSERT INTO flyway_schema_history VALUES (3,'3','v3','SQL','V3.sql',NULL,'test',now(),10,true)`)
	mustExec(t, db, `INSERT INTO flyway_schema_history VALUES (1,'1','v1','SQL','V1.sql',NULL,'test',now(),10,true)`)
	mustExec(t, db, `INSERT INTO flyway_schema_history VALUES (2,'2','v2','SQL','V2.sql',NULL,'test',now(),10,true)`)

	h, _ := schema.FetchMigrations(db, schema.ToolFlyway)
	if len(h.Migrations) != 3 {
		t.Fatalf("expected 3, got %d", len(h.Migrations))
	}
	if h.Migrations[0].Version != "1" || h.Migrations[2].Version != "3" {
		t.Errorf("wrong order: %v", []string{h.Migrations[0].Version, h.Migrations[1].Version, h.Migrations[2].Version})
	}
}

// ── Liquibase ────────────────────────────────────────────────────────────────

func TestFetchMigrations_Liquibase_Normal(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "databasechangelog") {
		t.Skip("databasechangelog already exists")
	}
	mustExec(t, db, liquibaseDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS databasechangelog") })

	mustExec(t, db, `INSERT INTO databasechangelog (id,author,filename,dateexecuted,orderexecuted,exectype,md5sum) VALUES ('1','dev','db.xml',now(),1,'EXECUTED','abc123')`)
	mustExec(t, db, `INSERT INTO databasechangelog (id,author,filename,dateexecuted,orderexecuted,exectype) VALUES ('2','dev','db.xml',now(),2,'EXECUTED')`)

	h, err := schema.FetchMigrations(db, schema.ToolLiquibase)
	if err != nil {
		t.Fatalf("FetchMigrations: %v", err)
	}
	if len(h.Migrations) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(h.Migrations))
	}
	if h.Migrations[0].Status != "success" {
		t.Errorf("expected success, got %s", h.Migrations[0].Status)
	}
}

func TestFetchMigrations_Liquibase_TableAbsent(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "databasechangelog") {
		t.Skip("databasechangelog exists; needs clean DB")
	}
	h, err := schema.FetchMigrations(db, schema.ToolLiquibase)
	if err != nil {
		t.Fatalf("must not error when table is absent: %v", err)
	}
	if len(h.Migrations) != 0 {
		t.Errorf("expected empty array, got %d", len(h.Migrations))
	}
}

func TestFetchMigrations_Liquibase_DuplicateRun(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "databasechangelog") {
		t.Skip("databasechangelog already exists")
	}
	mustExec(t, db, liquibaseDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS databasechangelog") })

	mustExec(t, db, `INSERT INTO databasechangelog (id,author,filename,dateexecuted,orderexecuted,exectype) VALUES ('1','dev','db.xml',now(),1,'EXECUTED')`)
	mustExec(t, db, `INSERT INTO databasechangelog (id,author,filename,dateexecuted,orderexecuted,exectype) VALUES ('1','dev','db.xml',now(),2,'RERAN')`)

	h, _ := schema.FetchMigrations(db, schema.ToolLiquibase)
	if len(h.Migrations) != 2 {
		t.Errorf("both executions must be returned, got %d", len(h.Migrations))
	}
	if h.Migrations[1].Status != "success" {
		t.Errorf("RERAN should map to success, got %s", h.Migrations[1].Status)
	}
}

// ── Prisma ───────────────────────────────────────────────────────────────────

func TestFetchMigrations_Prisma_Normal(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "_prisma_migrations") {
		t.Skip("_prisma_migrations already exists")
	}
	mustExec(t, db, prismaDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS _prisma_migrations") })

	now := time.Now().UTC()
	mustExec(t, db, fmt.Sprintf(`INSERT INTO _prisma_migrations (id,checksum,migration_name,started_at,finished_at,applied_steps_count) VALUES ('aaa','ccc','20240101_init','%s','%s',1)`,
		now.Add(-2*time.Second).Format(time.RFC3339), now.Format(time.RFC3339)))

	h, err := schema.FetchMigrations(db, schema.ToolPrisma)
	if err != nil {
		t.Fatalf("FetchMigrations: %v", err)
	}
	if len(h.Migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(h.Migrations))
	}
	if h.Migrations[0].Status != "success" {
		t.Errorf("expected success, got %s", h.Migrations[0].Status)
	}
}

func TestFetchMigrations_Prisma_TableAbsent(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "_prisma_migrations") {
		t.Skip("_prisma_migrations exists; needs clean DB")
	}
	h, err := schema.FetchMigrations(db, schema.ToolPrisma)
	if err != nil {
		t.Fatalf("must not error when table absent: %v", err)
	}
	if len(h.Migrations) != 0 {
		t.Errorf("expected empty array, got %d", len(h.Migrations))
	}
}

func TestFetchMigrations_Prisma_RolledBack(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "_prisma_migrations") {
		t.Skip("_prisma_migrations already exists")
	}
	mustExec(t, db, prismaDDL)
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS _prisma_migrations") })

	now := time.Now().UTC().Format(time.RFC3339)
	// finished_at is NULL → failed; rolled_back_at is set
	mustExec(t, db, fmt.Sprintf(`INSERT INTO _prisma_migrations (id,checksum,migration_name,started_at,rolled_back_at,applied_steps_count) VALUES ('bbb','ddd','20240102_bad','%s','%s',0)`, now, now))

	h, _ := schema.FetchMigrations(db, schema.ToolPrisma)
	if len(h.Migrations) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(h.Migrations))
	}
	if h.Migrations[0].Status != "failed" {
		t.Errorf("rolled-back migration should be failed, got %s", h.Migrations[0].Status)
	}
}

// ── FetchAllMigrations ───────────────────────────────────────────────────────

func TestFetchAllMigrations_NoTools(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") || tableExists(db, "databasechangelog") || tableExists(db, "_prisma_migrations") {
		t.Skip("migration tables present; needs clean DB")
	}
	histories, err := schema.FetchAllMigrations(db, "auto")
	if err != nil {
		t.Fatalf("FetchAllMigrations: %v", err)
	}
	if len(histories) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(histories))
	}
}

func TestFetchAllMigrations_BothTools(t *testing.T) {
	db := testDB(t)
	if tableExists(db, "flyway_schema_history") || tableExists(db, "databasechangelog") {
		t.Skip("migration tables present; needs clean DB")
	}
	mustExec(t, db, flywayDDL)
	mustExec(t, db, liquibaseDDL)
	t.Cleanup(func() {
		_, _ = db.Exec("DROP TABLE IF EXISTS flyway_schema_history")
		_, _ = db.Exec("DROP TABLE IF EXISTS databasechangelog")
	})

	histories, err := schema.FetchAllMigrations(db, "auto")
	if err != nil {
		t.Fatalf("FetchAllMigrations: %v", err)
	}
	if len(histories) < 2 {
		t.Fatalf("expected ≥2 histories, got %d", len(histories))
	}
	toolSet := make(map[string]bool)
	for _, h := range histories {
		toolSet[h.Tool] = true
	}
	if !toolSet["flyway"] || !toolSet["liquibase"] {
		t.Errorf("expected flyway and liquibase, got %v", toolSet)
	}
}
