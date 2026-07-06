package store

import "testing"

func seed(t *testing.T) *Store {
	t.Helper()
	st := openTest(t) // creates account acc-1
	if err := st.UpsertAccount(AccountRecord{UID: "acc-2", IBAN: "IT99", Name: "Savings", Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}
	txs := []TxRecord{
		{AccountUID: "acc-1", DedupKey: "1", Amount: -50, Currency: "EUR", CreditDebitIndicator: "DBIT", Status: "BOOK", BookingDate: "2026-06-10", Remittance: "ESSELUNGA", CreditorName: "Esselunga"},
		{AccountUID: "acc-1", DedupKey: "2", Amount: -30, Currency: "EUR", CreditDebitIndicator: "DBIT", Status: "BOOK", BookingDate: "2026-07-02", Remittance: "TRENITALIA"},
		{AccountUID: "acc-1", DedupKey: "3", Amount: 1500, Currency: "EUR", CreditDebitIndicator: "CRDT", Status: "BOOK", BookingDate: "2026-07-01", Remittance: "SALARY"},
		{AccountUID: "acc-2", DedupKey: "4", Amount: -20, Currency: "EUR", CreditDebitIndicator: "DBIT", Status: "BOOK", BookingDate: "2026-07-05", Remittance: "COOP"},
	}
	if _, err := st.UpsertTransactions(txs); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestListTransactionsFilters(t *testing.T) {
	st := seed(t)

	// All
	all, err := st.ListTransactions(TxFilter{})
	if err != nil || len(all) != 4 {
		t.Fatalf("all: n=%d err=%v", len(all), err)
	}
	// Newest first
	if all[0].BookingDate != "2026-07-05" {
		t.Errorf("expected newest first, got %s", all[0].BookingDate)
	}
	// By account
	byAcc, _ := st.ListTransactions(TxFilter{AccountUID: "acc-2"})
	if len(byAcc) != 1 || byAcc[0].Remittance != "COOP" {
		t.Errorf("account filter failed: %+v", byAcc)
	}
	// Date range (July only)
	july, _ := st.ListTransactions(TxFilter{DateFrom: "2026-07-01", DateTo: "2026-07-31"})
	if len(july) != 3 {
		t.Errorf("july range: n=%d", len(july))
	}
	// Direction
	credits, _ := st.ListTransactions(TxFilter{Direction: "CRDT"})
	if len(credits) != 1 || credits[0].Amount != 1500 {
		t.Errorf("direction filter failed: %+v", credits)
	}
	// Text (case-insensitive)
	text, _ := st.ListTransactions(TxFilter{Text: "esselunga"})
	if len(text) != 1 || text[0].Amount != -50 {
		t.Errorf("text filter failed: %+v", text)
	}
	// Count ignores limit/offset
	n, _ := st.CountTransactions(TxFilter{DateFrom: "2026-07-01", DateTo: "2026-07-31"})
	if n != 3 {
		t.Errorf("count = %d, want 3", n)
	}
}

func TestSumByMonth(t *testing.T) {
	st := seed(t)
	sums, err := st.SumByMonth("2026-01-01", "2026-12-31")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]MonthSum{}
	for _, s := range sums {
		got[s.Month] = s
	}
	if got["2026-06"].Out != 50 {
		t.Errorf("June out = %v, want 50", got["2026-06"].Out)
	}
	if got["2026-07"].In != 1500 {
		t.Errorf("July in = %v, want 1500", got["2026-07"].In)
	}
	if got["2026-07"].Out != 50 { // 30 + 20
		t.Errorf("July out = %v, want 50", got["2026-07"].Out)
	}
}

func TestListAccountsAndBalances(t *testing.T) {
	st := seed(t)
	accs, err := st.ListAccounts()
	if err != nil || len(accs) != 2 {
		t.Fatalf("accounts: n=%d err=%v", len(accs), err)
	}
	if err := st.UpsertBalance(BalanceRecord{AccountUID: "acc-1", BalanceType: "CLBD", Amount: 1420.50, Currency: "EUR", ReferenceDate: "2026-07-06"}); err != nil {
		t.Fatal(err)
	}
	// upsert same type updates
	if err := st.UpsertBalance(BalanceRecord{AccountUID: "acc-1", BalanceType: "CLBD", Amount: 1500, Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}
	bals, _ := st.ListBalances()
	if len(bals) != 1 || bals[0].Amount != 1500 {
		t.Errorf("balances: %+v", bals)
	}
}
