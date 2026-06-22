package schema

import "time"

type Column struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default,omitempty"`
}

type ForeignKey struct {
	Column           string `json:"column"`
	ReferencedTable  string `json:"referenced_table"`
	ReferencedColumn string `json:"referenced_column"`
}

type Table struct {
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	PrimaryKeys []string     `json:"primary_keys"`
	ForeignKeys []ForeignKey `json:"foreign_keys"`
}

type ERD struct {
	Tables []Table `json:"tables"`
}

type Migration struct {
	Version     string    `json:"version"`
	Description string    `json:"description"`
	AppliedAt   time.Time `json:"applied_at"`
	Status      string    `json:"status"` // "success" | "failed"
	Checksum    *string   `json:"checksum,omitempty"`
}

type MigrationHistory struct {
	Tool       string      `json:"tool"`
	Migrations []Migration `json:"migrations"`
}

type ColumnChange struct {
	Name           string  `json:"name"`
	OldType        string  `json:"old_type,omitempty"`
	NewType        string  `json:"new_type,omitempty"`
	OldNullable    *bool   `json:"old_nullable,omitempty"` // non-nil only when nullability changed
	NewNullable    *bool   `json:"new_nullable,omitempty"`
	DefaultChanged bool    `json:"default_changed,omitempty"`
	OldDefault     *string `json:"old_default,omitempty"` // nil means no default was set
	NewDefault     *string `json:"new_default,omitempty"`
}

type TableDiff struct {
	Table              string         `json:"table"`
	AddedColumns       []Column       `json:"added_columns,omitempty"`
	RemovedColumns     []Column       `json:"removed_columns,omitempty"`
	ModifiedColumns    []ColumnChange `json:"modified_columns,omitempty"`
	AddedConstraints   []string       `json:"added_constraints,omitempty"`
	RemovedConstraints []string       `json:"removed_constraints,omitempty"`
	AddedForeignKeys   []ForeignKey   `json:"added_foreign_keys,omitempty"`
	RemovedForeignKeys []ForeignKey   `json:"removed_foreign_keys,omitempty"`
}

type SchemaDiff struct {
	From     string      `json:"from"`
	To       string      `json:"to"`
	Added    []string    `json:"added"`
	Removed  []string    `json:"removed"`
	Modified []TableDiff `json:"modified"`
}
