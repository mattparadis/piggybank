package store

import (
	"time"
)

// AccountRecord is a row of the accounts table.
type AccountRecord struct {
	UID      string
	IBAN     string
	Name     string
	Currency string
	Product  string
	RawJSON  string
}

// TxRecord is a row of the transactions table. The caller (syncer) computes
// DedupKey and Amount (signed).
type TxRecord struct {
	AccountUID           string
	DedupKey             string
	TransactionID        string
	Amount               float64
	Currency             string
	CreditDebitIndicator string
	Status               string
	BookingDate          string
	ValueDate            string
	TransactionDate      string
	Reference            string
	Remittance           string
	CreditorName         string
	DebtorName           string
	RawJSON              string
}

// SyncState tracks the incremental synchronization state per account.
type SyncState struct {
	AccountUID      string
	LastSyncedAt    string
	LastBookingDate string
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// UpsertAccount inserts or updates an account.
func (s *Store) UpsertAccount(a AccountRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO accounts (account_uid, iban, name, currency, product, raw_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_uid) DO UPDATE SET
			iban=excluded.iban,
			name=excluded.name,
			currency=excluded.currency,
			product=excluded.product,
			raw_json=excluded.raw_json,
			updated_at=excluded.updated_at`,
		a.UID, a.IBAN, a.Name, a.Currency, a.Product, a.RawJSON, nowUTC())
	return err
}

// UpsertTransactions inserts transactions idempotently (key
// account_uid + dedup_key) and updates those already present (e.g. status PDNG->BOOK).
// Returns the number of genuinely new transactions (useful for notifications).
func (s *Store) UpsertTransactions(txs []TxRecord) (int, error) {
	if len(txs) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	insert, err := tx.Prepare(`
		INSERT INTO transactions (
			account_uid, dedup_key, transaction_id, amount, currency,
			credit_debit_indicator, status, booking_date, value_date,
			transaction_date, reference, remittance_information,
			creditor_name, debtor_name, raw_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_uid, dedup_key) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer insert.Close()

	update, err := tx.Prepare(`
		UPDATE transactions SET
			transaction_id=?, amount=?, status=?, booking_date=?, value_date=?,
			transaction_date=?, reference=?, remittance_information=?,
			creditor_name=?, debtor_name=?, raw_json=?
		WHERE account_uid=? AND dedup_key=?`)
	if err != nil {
		return 0, err
	}
	defer update.Close()

	now := nowUTC()
	newCount := 0
	for _, t := range txs {
		res, err := insert.Exec(
			t.AccountUID, t.DedupKey, t.TransactionID, t.Amount, t.Currency,
			t.CreditDebitIndicator, t.Status, t.BookingDate, t.ValueDate,
			t.TransactionDate, t.Reference, t.Remittance,
			t.CreditorName, t.DebtorName, t.RawJSON, now)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			newCount++
			continue
		}
		// Already present: update the fields that can change.
		if _, err := update.Exec(
			t.TransactionID, t.Amount, t.Status, t.BookingDate, t.ValueDate,
			t.TransactionDate, t.Reference, t.Remittance,
			t.CreditorName, t.DebtorName, t.RawJSON,
			t.AccountUID, t.DedupKey); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return newCount, nil
}

// GetSyncState returns the sync state of an account (zero-value if absent).
func (s *Store) GetSyncState(accountUID string) (SyncState, error) {
	st := SyncState{AccountUID: accountUID}
	row := s.db.QueryRow(
		`SELECT last_synced_at, last_booking_date FROM sync_state WHERE account_uid=?`,
		accountUID)
	var synced, booking *string
	switch err := row.Scan(&synced, &booking); err {
	case nil:
		if synced != nil {
			st.LastSyncedAt = *synced
		}
		if booking != nil {
			st.LastBookingDate = *booking
		}
		return st, nil
	default:
		if err.Error() == "sql: no rows in result set" {
			return st, nil
		}
		return st, err
	}
}

// SetSyncState saves the sync state of an account.
func (s *Store) SetSyncState(st SyncState) error {
	_, err := s.db.Exec(`
		INSERT INTO sync_state (account_uid, last_synced_at, last_booking_date)
		VALUES (?, ?, ?)
		ON CONFLICT(account_uid) DO UPDATE SET
			last_synced_at=excluded.last_synced_at,
			last_booking_date=excluded.last_booking_date`,
		st.AccountUID, st.LastSyncedAt, st.LastBookingDate)
	return err
}
