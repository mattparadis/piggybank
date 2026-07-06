// Package store persists accounts and transactions in SQLite (pure-Go driver
// modernc.org/sqlite, no cgo).
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Store wraps the database connection.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the database and runs the migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the connection.
func (s *Store) Close() error {
	return s.db.Close()
}
