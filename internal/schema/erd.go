package schema

import (
	"context"
	"database/sql"
	"time"
)

func ExtractERD(db *sql.DB) (*ERD, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tables, err := fetchTables(ctx, db)
	if err != nil {
		return nil, err
	}

	for i := range tables {
		tables[i].Columns, err = fetchColumns(ctx, db, tables[i].Name)
		if err != nil {
			return nil, err
		}
		tables[i].PrimaryKeys, err = fetchPrimaryKeys(ctx, db, tables[i].Name)
		if err != nil {
			return nil, err
		}
		tables[i].ForeignKeys, err = fetchForeignKeys(ctx, db, tables[i].Name)
		if err != nil {
			return nil, err
		}
	}

	return &ERD{Tables: tables}, nil
}

func fetchTables(ctx context.Context, db *sql.DB) ([]Table, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []Table
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.Name); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	if tables == nil {
		tables = []Table{}
	}
	return tables, rows.Err()
}

func fetchColumns(ctx context.Context, db *sql.DB, table string) ([]Column, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1
		ORDER BY ordinal_position
	`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []Column
	for rows.Next() {
		var c Column
		var nullable string
		var def sql.NullString
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &def); err != nil {
			return nil, err
		}
		c.Nullable = nullable == "YES"
		if def.Valid {
			c.Default = &def.String
		}
		cols = append(cols, c)
	}
	if cols == nil {
		cols = []Column{}
	}
	return cols, rows.Err()
}

func fetchPrimaryKeys(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema   = kcu.table_schema
		WHERE tc.constraint_type = 'PRIMARY KEY'
		  AND tc.table_schema    = 'public'
		  AND tc.table_name      = $1
		ORDER BY kcu.ordinal_position
	`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pks []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		pks = append(pks, col)
	}
	if pks == nil {
		pks = []string{}
	}
	return pks, rows.Err()
}

func fetchForeignKeys(ctx context.Context, db *sql.DB, table string) ([]ForeignKey, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			kcu.column_name,
			ccu.table_name  AS referenced_table,
			ccu.column_name AS referenced_column
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name  = kcu.constraint_name
		 AND tc.table_schema     = kcu.table_schema
		JOIN information_schema.referential_constraints rc
		  ON tc.constraint_name  = rc.constraint_name
		 AND tc.table_schema     = rc.constraint_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON rc.unique_constraint_name   = ccu.constraint_name
		 AND rc.unique_constraint_schema = ccu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_schema    = 'public'
		  AND tc.table_name      = $1
		ORDER BY kcu.column_name
	`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fks []ForeignKey
	for rows.Next() {
		var fk ForeignKey
		if err := rows.Scan(&fk.Column, &fk.ReferencedTable, &fk.ReferencedColumn); err != nil {
			return nil, err
		}
		fks = append(fks, fk)
	}
	if fks == nil {
		fks = []ForeignKey{}
	}
	return fks, rows.Err()
}
