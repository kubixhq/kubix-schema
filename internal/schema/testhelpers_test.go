package schema_test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/kubixhq/kubix-schema/internal/schema"
)

// testDB returns a live *sql.DB or skips the test if TEST_DB_DSN is not set.
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

// testPrefix returns a unique, SQL-safe identifier prefix for this test.
func testPrefix(t *testing.T) string {
	t.Helper()
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, strings.ToLower(t.Name()))
	if len(safe) > 12 {
		safe = safe[len(safe)-12:]
	}
	return fmt.Sprintf("t%s%04x_", safe, rand.N(uint(0xFFFF)))
}

func mustExec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("exec failed: %v\n  query: %s", err, query)
	}
}

func dropTable(t *testing.T, db *sql.DB, name string) {
	t.Cleanup(func() { db.Exec("DROP TABLE IF EXISTS " + name + " CASCADE") })
}

func dropView(t *testing.T, db *sql.DB, name string) {
	t.Cleanup(func() { db.Exec("DROP VIEW IF EXISTS " + name + " CASCADE") })
}

func dropSchema(t *testing.T, db *sql.DB, name string) {
	t.Cleanup(func() { db.Exec("DROP SCHEMA IF EXISTS " + name + " CASCADE") })
}

func tableExists(db *sql.DB, name string) bool {
	var exists bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`, name).Scan(&exists)
	return exists
}

func findTable(erd *schema.ERD, name string) *schema.Table {
	for i := range erd.Tables {
		if erd.Tables[i].Name == name {
			return &erd.Tables[i]
		}
	}
	return nil
}

func colByName(tbl *schema.Table, name string) *schema.Column {
	for i := range tbl.Columns {
		if tbl.Columns[i].Name == name {
			return &tbl.Columns[i]
		}
	}
	return nil
}
