package schema

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadSnapshot returns the ERD for a named version.
// "current" is a reserved keyword that returns the live schema from the database.
// Any other string loads a previously stored snapshot from snapshotDir.
func LoadSnapshot(db *sql.DB, version, snapshotDir string) (*ERD, error) {
	if version == "current" {
		return ExtractERD(db)
	}
	path := filepath.Join(snapshotDir, sanitizeFilename(version)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot not found: %s", version)
		}
		return nil, err
	}
	var erd ERD
	if err := json.Unmarshal(data, &erd); err != nil {
		return nil, fmt.Errorf("corrupt snapshot %s: %w", version, err)
	}
	return &erd, nil
}

// SaveSnapshot writes an ERD snapshot to disk under snapshotDir/<version>.json.
func SaveSnapshot(erd *ERD, version, snapshotDir string) error {
	data, err := json.MarshalIndent(erd, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(snapshotDir, sanitizeFilename(version)+".json")
	return os.WriteFile(path, data, 0644)
}

// CaptureStartupSnapshot captures the current schema and tags it with the latest
// migration version (falls back to "startup" when no migrations are found).
func CaptureStartupSnapshot(db *sql.DB, tool Tool, snapshotDir string) error {
	erd, err := ExtractERD(db)
	if err != nil {
		return err
	}
	version := "startup"
	if history, err := FetchMigrations(db, tool); err == nil && len(history.Migrations) > 0 {
		last := history.Migrations[len(history.Migrations)-1]
		if last.Version != "" {
			version = last.Version
		}
	}
	return SaveSnapshot(erd, version, snapshotDir)
}

// Diff computes the structural difference between two ERD snapshots.
func Diff(from, to *ERD, fromVersion, toVersion string) *SchemaDiff {
	d := &SchemaDiff{
		From:     fromVersion,
		To:       toVersion,
		Added:    []string{},
		Removed:  []string{},
		Modified: []TableDiff{},
	}

	fromMap := tableIndex(from)
	toMap := tableIndex(to)

	for name := range toMap {
		if _, ok := fromMap[name]; !ok {
			d.Added = append(d.Added, name)
		}
	}
	for name := range fromMap {
		if _, ok := toMap[name]; !ok {
			d.Removed = append(d.Removed, name)
		}
	}
	for name, toTable := range toMap {
		fromTable, ok := fromMap[name]
		if !ok {
			continue
		}
		if td := diffTable(fromTable, toTable); td != nil {
			d.Modified = append(d.Modified, *td)
		}
	}
	return d
}

func diffTable(from, to Table) *TableDiff {
	td := &TableDiff{Table: to.Name}
	changed := false

	fromCols := colIndex(from.Columns)
	toCols := colIndex(to.Columns)

	for name, col := range toCols {
		if _, ok := fromCols[name]; !ok {
			td.AddedColumns = append(td.AddedColumns, col)
			changed = true
		}
	}
	for name, col := range fromCols {
		if _, ok := toCols[name]; !ok {
			td.RemovedColumns = append(td.RemovedColumns, col)
			changed = true
		}
	}
	for name, toCol := range toCols {
		fromCol, ok := fromCols[name]
		if !ok {
			continue
		}
		if fromCol.Type != toCol.Type {
			td.ModifiedColumns = append(td.ModifiedColumns, ColumnChange{
				Name:    name,
				OldType: fromCol.Type,
				NewType: toCol.Type,
			})
			changed = true
		}
	}

	fromPKs := strSet(from.PrimaryKeys)
	toPKs := strSet(to.PrimaryKeys)
	for pk := range toPKs {
		if !fromPKs[pk] {
			td.AddedConstraints = append(td.AddedConstraints, "PRIMARY KEY("+pk+")")
			changed = true
		}
	}
	for pk := range fromPKs {
		if !toPKs[pk] {
			td.RemovedConstraints = append(td.RemovedConstraints, "PRIMARY KEY("+pk+")")
			changed = true
		}
	}

	fromFKs := fkIndex(from.ForeignKeys)
	toFKs := fkIndex(to.ForeignKeys)
	for key, fk := range toFKs {
		if _, ok := fromFKs[key]; !ok {
			td.AddedForeignKeys = append(td.AddedForeignKeys, fk)
			changed = true
		}
	}
	for key, fk := range fromFKs {
		if _, ok := toFKs[key]; !ok {
			td.RemovedForeignKeys = append(td.RemovedForeignKeys, fk)
			changed = true
		}
	}

	if !changed {
		return nil
	}
	return td
}

func tableIndex(erd *ERD) map[string]Table {
	m := make(map[string]Table, len(erd.Tables))
	for _, t := range erd.Tables {
		m[t.Name] = t
	}
	return m
}

func colIndex(cols []Column) map[string]Column {
	m := make(map[string]Column, len(cols))
	for _, c := range cols {
		m[c.Name] = c
	}
	return m
}

func strSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

func fkIndex(fks []ForeignKey) map[string]ForeignKey {
	m := make(map[string]ForeignKey, len(fks))
	for _, fk := range fks {
		key := fk.Column + "->" + fk.ReferencedTable + "." + fk.ReferencedColumn
		m[key] = fk
	}
	return m
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}
