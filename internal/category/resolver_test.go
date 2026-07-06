package category

import (
	"testing"

	"expense_monitor/internal/config"
	"expense_monitor/internal/store"
)

func resolverTestCat() *Categorizer {
	return Build([]config.Category{
		{Name: "Groceries", MatchAny: []string{"COOP"}},
		{Name: "Dining", MatchAny: []string{"KFC"}},
		{Name: "Savings", Savings: true, MatchAny: []string{"GIROCONTO"}},
	}, nil)
}

func TestResolverPriority(t *testing.T) {
	cat := resolverTestCat()
	tx := store.TxRecord{ID: 1, Remittance: "DEL 01/07 C/O KFC"}

	// Base keyword rule: KFC -> Dining.
	if got := cat.NewResolver(nil, nil).Resolve(tx); got.Name != "Dining" {
		t.Errorf("base resolve = %q, want Dining", got.Name)
	}
	// Learned rule beats the base keyword rule.
	learned := cat.NewResolver(nil, []LearnedRule{{Keyword: "kfc", Category: "Groceries"}})
	if got := learned.Resolve(tx); got.Name != "Groceries" {
		t.Errorf("learned resolve = %q, want Groceries", got.Name)
	}
	// Per-transaction override beats everything.
	both := cat.NewResolver(map[int64]string{1: "Savings"}, []LearnedRule{{Keyword: "kfc", Category: "Groceries"}})
	got := both.Resolve(tx)
	if got.Name != "Savings" || !got.Savings {
		t.Errorf("override resolve = %q (savings=%v), want Savings (savings=true)", got.Name, got.Savings)
	}
}

func TestResolverSummarizeOverrideToSavings(t *testing.T) {
	cat := resolverTestCat()
	txs := []store.TxRecord{{ID: 1, Amount: -40, Remittance: "KFC"}}

	base := cat.NewResolver(nil, nil).Summarize(txs)
	if base.Spent != 40 || base.Saved != 0 {
		t.Fatalf("base: spent=%v saved=%v, want 40/0", base.Spent, base.Saved)
	}
	// Overriding the tx into the savings category moves it out of Spent.
	over := cat.NewResolver(map[int64]string{1: "Savings"}, nil).Summarize(txs)
	if over.Spent != 0 || over.Saved != 40 {
		t.Fatalf("override: spent=%v saved=%v, want 0/40", over.Spent, over.Saved)
	}
}
