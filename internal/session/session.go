// Package session persiste la sessione Enable Banking (session_id + scadenza +
// account autorizzati) su file JSON, condiviso tra il comando `auth` e il daemon.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Account è la versione minimale e stabile di un conto autorizzato.
type Account struct {
	UID      string `json:"uid"`
	IBAN     string `json:"iban"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// Session è ciò che viene salvato in session.json.
type Session struct {
	SessionID  string    `json:"session_id"`
	ValidUntil time.Time `json:"valid_until"`
	Accounts   []Account `json:"accounts"`
	CreatedAt  time.Time `json:"created_at"`
}

// Expired indica se il consenso è scaduto secondo valid_until.
func (s *Session) Expired() bool {
	return !s.ValidUntil.IsZero() && time.Now().After(s.ValidUntil)
}

// Load legge session.json.
func Load(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &s, nil
}

// Save scrive session.json con permessi restrittivi (contiene un riferimento
// alla sessione bancaria).
func Save(path string, s *Session) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
