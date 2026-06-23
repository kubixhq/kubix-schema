package schema

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Tool string

const (
	ToolFlyway    Tool = "flyway"
	ToolLiquibase Tool = "liquibase"
	ToolPrisma    Tool = "prisma"
	ToolUnknown   Tool = "unknown"
)

// DetectAllMigrationTools returns every migration tool present in the database.
// When hint names a specific tool it returns only that one (present or not).
func DetectAllMigrationTools(db *sql.DB, hint string) []Tool {
	if hint != "" && hint != "auto" {
		return []Tool{Tool(hint)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var tools []Tool
	for _, candidate := range []struct {
		tool  Tool
		table string
	}{
		{ToolFlyway, "flyway_schema_history"},
		{ToolLiquibase, "databasechangelog"},
		{ToolPrisma, "_prisma_migrations"},
	} {
		var exists bool
		_ = db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)
		`, candidate.table).Scan(&exists)
		if exists {
			tools = append(tools, candidate.tool)
		}
	}
	return tools
}

// FetchAllMigrations returns history for every detected migration tool.
// Returns an empty slice (not an error) when no tool is found.
// Returns an error when the database itself is unavailable.
func FetchAllMigrations(db *sql.DB, hint string) ([]MigrationHistory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	tools := DetectAllMigrationTools(db, hint)
	if len(tools) == 0 {
		return []MigrationHistory{}, nil
	}
	results := make([]MigrationHistory, 0, len(tools))
	for _, tool := range tools {
		h, err := FetchMigrations(db, tool)
		if err != nil {
			return nil, err
		}
		results = append(results, *h)
	}
	return results, nil
}

// DetectMigrationTool returns the tool specified by hint, or probes the DB when hint is "auto".
func DetectMigrationTool(db *sql.DB, hint string) Tool {
	if hint != "" && hint != "auto" {
		return Tool(hint)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, candidate := range []struct {
		tool  Tool
		table string
	}{
		{ToolFlyway, "flyway_schema_history"},
		{ToolLiquibase, "databasechangelog"},
		{ToolPrisma, "_prisma_migrations"},
	} {
		var exists bool
		_ = db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)
		`, candidate.table).Scan(&exists)
		if exists {
			return candidate.tool
		}
	}
	return ToolUnknown
}

// FetchMigrations returns the migration history for the detected tool.
// Returns an empty list (not an error) when the migration table is absent.
func FetchMigrations(db *sql.DB, tool Tool) (*MigrationHistory, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch tool {
	case ToolFlyway:
		return fetchFlyway(ctx, db)
	case ToolLiquibase:
		return fetchLiquibase(ctx, db)
	case ToolPrisma:
		return fetchPrisma(ctx, db)
	default:
		return &MigrationHistory{Tool: string(tool), Migrations: []Migration{}}, nil
	}
}

func fetchFlyway(ctx context.Context, db *sql.DB) (*MigrationHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT version, description, installed_on, success, checksum,
		       COALESCE(execution_time, 0),
		       COALESCE(script, ''),
		       COALESCE(installed_by, '')
		FROM flyway_schema_history
		ORDER BY installed_rank
	`)
	if err != nil {
		// table absent or inaccessible — return empty, not an error
		return &MigrationHistory{Tool: "flyway", Migrations: []Migration{}}, nil
	}
	defer rows.Close()

	var migrations []Migration
	for rows.Next() {
		var version, desc, script, installedBy sql.NullString
		var appliedAt time.Time
		var success bool
		var checksum sql.NullInt64
		var execTime int64
		if err := rows.Scan(&version, &desc, &appliedAt, &success, &checksum, &execTime, &script, &installedBy); err != nil {
			return nil, err
		}
		m := Migration{
			Version:       version.String,
			Description:   desc.String,
			AppliedAt:     appliedAt,
			ExecutionTime: execTime,
			Script:        script.String,
			InstalledBy:   installedBy.String,
		}
		if success {
			m.Status = "success"
		} else {
			m.Status = "failed"
		}
		if checksum.Valid {
			s := fmt.Sprintf("%d", checksum.Int64)
			m.Checksum = &s
		}
		migrations = append(migrations, m)
	}
	if migrations == nil {
		migrations = []Migration{}
	}
	return &MigrationHistory{Tool: "flyway", Migrations: migrations}, rows.Err()
}

func fetchLiquibase(ctx context.Context, db *sql.DB) (*MigrationHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, description, dateexecuted, exectype, md5sum
		FROM databasechangelog
		ORDER BY orderexecuted
	`)
	if err != nil {
		return &MigrationHistory{Tool: "liquibase", Migrations: []Migration{}}, nil
	}
	defer rows.Close()

	var migrations []Migration
	for rows.Next() {
		var id, desc, execType, md5 sql.NullString
		var appliedAt time.Time
		if err := rows.Scan(&id, &desc, &appliedAt, &execType, &md5); err != nil {
			return nil, err
		}
		m := Migration{
			Version:     id.String,
			Description: desc.String,
			AppliedAt:   appliedAt,
		}
		if execType.String == "EXECUTED" || execType.String == "RERAN" {
			m.Status = "success"
		} else {
			m.Status = "failed"
		}
		if md5.Valid {
			m.Checksum = &md5.String
		}
		migrations = append(migrations, m)
	}
	if migrations == nil {
		migrations = []Migration{}
	}
	return &MigrationHistory{Tool: "liquibase", Migrations: migrations}, rows.Err()
}

func fetchPrisma(ctx context.Context, db *sql.DB) (*MigrationHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT migration_name, started_at, finished_at, applied_steps_count, checksum
		FROM _prisma_migrations
		ORDER BY started_at
	`)
	if err != nil {
		return &MigrationHistory{Tool: "prisma", Migrations: []Migration{}}, nil
	}
	defer rows.Close()

	var migrations []Migration
	for rows.Next() {
		var name, checksum sql.NullString
		var startedAt time.Time
		var finishedAt sql.NullTime
		var steps sql.NullInt64
		if err := rows.Scan(&name, &startedAt, &finishedAt, &steps, &checksum); err != nil {
			return nil, err
		}
		m := Migration{
			Version:     name.String,
			Description: name.String,
			AppliedAt:   startedAt,
		}
		if finishedAt.Valid {
			m.Status = "success"
		} else {
			m.Status = "failed"
		}
		if checksum.Valid {
			m.Checksum = &checksum.String
		}
		migrations = append(migrations, m)
	}
	if migrations == nil {
		migrations = []Migration{}
	}
	return &MigrationHistory{Tool: "prisma", Migrations: migrations}, rows.Err()
}
