package store

import "testing"

func TestCategoryOverrideRoundTrip(t *testing.T) {
	st := openTest(t)
	if _, err := st.UpsertTransactions([]TxRecord{
		{AccountUID: "acc-1", DedupKey: "k1", Amount: -10, BookingDate: "2026-07-01", Remittance: "KFC"},
	}); err != nil {
		t.Fatal(err)
	}
	txs, err := st.ListTransactions(TxFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 1 || txs[0].ID == 0 {
		t.Fatalf("expected 1 tx with non-zero ID, got %+v", txs)
	}
	id := txs[0].ID

	// GetTransaction returns the same row.
	got, err := st.GetTransaction(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != id || got.Remittance != "KFC" {
		t.Fatalf("GetTransaction = %+v", got)
	}

	// Override set/list/delete.
	if err := st.SetCategoryOverride(id, "Dining"); err != nil {
		t.Fatal(err)
	}
	ov, err := st.ListCategoryOverrides()
	if err != nil {
		t.Fatal(err)
	}
	if ov[id] != "Dining" {
		t.Fatalf("override = %q, want Dining", ov[id])
	}
	// Upsert replaces.
	if err := st.SetCategoryOverride(id, "Groceries"); err != nil {
		t.Fatal(err)
	}
	ov, _ = st.ListCategoryOverrides()
	if ov[id] != "Groceries" {
		t.Fatalf("override after update = %q, want Groceries", ov[id])
	}
	if err := st.DeleteCategoryOverride(id); err != nil {
		t.Fatal(err)
	}
	ov, _ = st.ListCategoryOverrides()
	if _, ok := ov[id]; ok {
		t.Fatalf("override should be deleted")
	}
}

func TestLearnedRulesRoundTrip(t *testing.T) {
	st := openTest(t)
	if err := st.AddLearnedRule("KFC", "Dining"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddLearnedRule("COOP", "Groceries"); err != nil {
		t.Fatal(err)
	}
	rules, err := st.ListLearnedRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(rules))
	}
	// Newest first.
	if rules[0].Keyword != "COOP" {
		t.Errorf("first rule = %q, want COOP", rules[0].Keyword)
	}
	if err := st.DeleteLearnedRule(rules[0].ID); err != nil {
		t.Fatal(err)
	}
	rules, _ = st.ListLearnedRules()
	if len(rules) != 1 || rules[0].Keyword != "KFC" {
		t.Fatalf("after delete = %+v", rules)
	}
}
