// Package store persiste conti e transazioni in SQLite (driver pure-Go
// modernc.org/sqlite, nessun cgo).
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Store incapsula la connessione al database.
type Store struct {
	db *sql.DB
}

// Open apre (o crea) il database ed esegue le migrazioni.
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

// Close chiude la connessione.
func (s *Store) Close() error {
	return s.db.Close()
}
