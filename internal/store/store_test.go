package store

import (
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.UpsertAccount(AccountRecord{UID: "acc-1", IBAN: "IT00", Name: "Conto", Currency: "EUR"}); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}
	return st
}

func TestUpsertTransactionsIsIdempotent(t *testing.T) {
	st := openTest(t)

	txs := []TxRecord{
		{AccountUID: "acc-1", DedupKey: "id:t1", TransactionID: "t1", Amount: -10, Currency: "EUR", Status: "BOOK", BookingDate: "2026-07-01"},
		{AccountUID: "acc-1", DedupKey: "id:t2", TransactionID: "t2", Amount: 20, Currency: "EUR", Status: "BOOK", BookingDate: "2026-07-02"},
	}

	// First upsert: two new.
	n, err := st.UpsertTransactions(txs)
	if err != nil {
		t.Fatalf("UpsertTransactions: %v", err)
	}
	if n != 2 {
		t.Fatalf("new transactions = %d, want 2", n)
	}

	// Second identical upsert: zero new.
	n, err = st.UpsertTransactions(txs)
	if err != nil {
		t.Fatalf("UpsertTransactions (2): %v", err)
	}
	if n != 0 {
		t.Errorf("new transactions on re-upsert = %d, want 0", n)
	}

	if got := st.countTx(t); got != 2 {
		t.Errorf("total rows = %d, want 2 (no duplicates)", got)
	}
}

func TestUpsertUpdatesStatus(t *testing.T) {
	st := openTest(t)

	pending := []TxRecord{{AccountUID: "acc-1", DedupKey: "id:t1", TransactionID: "t1", Amount: -5, Status: "PDNG", BookingDate: "2026-07-01"}}
	if _, err := st.UpsertTransactions(pending); err != nil {
		t.Fatal(err)
	}
	booked := []TxRecord{{AccountUID: "acc-1", DedupKey: "id:t1", TransactionID: "t1", Amount: -5, Status: "BOOK", BookingDate: "2026-07-01"}}
	n, err := st.UpsertTransactions(booked)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("the update must not count as new: n=%d", n)
	}
	if got := st.statusOf(t, "id:t1"); got != "BOOK" {
		t.Errorf("status = %q, want BOOK", got)
	}
}

func TestSyncStateRoundTrip(t *testing.T) {
	st := openTest(t)
	if err := st.SetSyncState(SyncState{AccountUID: "acc-1", LastSyncedAt: "2026-07-06T00:00:00Z", LastBookingDate: "2026-07-05"}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetSyncState("acc-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastBookingDate != "2026-07-05" {
		t.Errorf("LastBookingDate = %q", got.LastBookingDate)
	}

	// Account without state: zero-value with no error.
	empty, err := st.GetSyncState("assente")
	if err != nil {
		t.Fatalf("GetSyncState missing: %v", err)
	}
	if empty.LastBookingDate != "" {
		t.Errorf("expected empty state, got %q", empty.LastBookingDate)
	}
}

func (s *Store) countTx(t *testing.T) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM transactions`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (s *Store) statusOf(t *testing.T, dedup string) string {
	t.Helper()
	var status string
	if err := s.db.QueryRow(`SELECT status FROM transactions WHERE dedup_key=?`, dedup).Scan(&status); err != nil {
		t.Fatalf("statusOf: %v", err)
	}
	return status
}
