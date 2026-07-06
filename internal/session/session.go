// Package session persists the Enable Banking session (session_id + expiry +
// authorized accounts) to a JSON file, shared between the `auth` command and the daemon.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Account is the minimal, stable representation of an authorized account.
type Account struct {
	UID      string `json:"uid"`
	IBAN     string `json:"iban"`
	Name     string `json:"name"`
	Currency string `json:"currency"`
}

// Session is what gets saved in session.json.
type Session struct {
	SessionID  string    `json:"session_id"`
	ValidUntil time.Time `json:"valid_until"`
	Accounts   []Account `json:"accounts"`
	CreatedAt  time.Time `json:"created_at"`
}

// Expired reports whether the consent has expired according to valid_until.
func (s *Session) Expired() bool {
	return !s.ValidUntil.IsZero() && time.Now().After(s.ValidUntil)
}

// Load reads session.json.
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

// Save writes session.json with restrictive permissions (it contains a
// reference to the banking session).
func Save(path string, s *Session) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
