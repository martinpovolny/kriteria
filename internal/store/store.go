package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Store wraps the SQLite database handle.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite serial writes
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying *sql.DB for use by API handlers.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return err
	}
	return s.migrateColumns(ctx)
}

// migrateColumns adds columns that may not exist in older DBs.
// Uses pragma_table_info to check before adding (idempotent).
func (s *Store) migrateColumns(ctx context.Context) error {
	addIfMissing := func(table, column, decl string) error {
		var count int
		err := s.db.QueryRowContext(ctx,
			`SELECT count(*) FROM pragma_table_info(?) WHERE name = ?`,
			table, column).Scan(&count)
		if err != nil {
			return fmt.Errorf("check column %s.%s: %w", table, column, err)
		}
		if count == 0 {
			_, err := s.db.ExecContext(ctx,
				fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, decl))
			if err != nil {
				return fmt.Errorf("add column %s.%s: %w", table, column, err)
			}
		}
		return nil
	}

	if err := addIfMissing("student", "person_id", "INTEGER"); err != nil {
		return err
	}
	if err := addIfMissing("student", "current_grade", "INTEGER"); err != nil {
		return err
	}
	return nil
}
