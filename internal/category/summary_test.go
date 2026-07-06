package category

import (
	"testing"

	"expense_monitor/internal/config"
	"expense_monitor/internal/store"
)

func TestSummarizeExcludesSavings(t *testing.T) {
	cat := Build([]config.Category{
		{Name: "Groceries", MatchAny: []string{"COOP"}},
		{Name: "Savings", Savings: true, MatchAny: []string{"GIROCONTO"}},
	}, nil)

	txs := []store.TxRecord{
		{Amount: -30, Remittance: "COOP store"},         // spending: Groceries
		{Amount: -100, Remittance: "GIROCONTO to bank"}, // savings (excluded from spend)
		{Amount: -20, Remittance: "misc shop"},          // spending: Uncategorized
		{Amount: 200, Remittance: "SALARY"},             // income
	}

	sum := cat.NewResolver(nil, nil).Summarize(txs)
	if sum.Spent != 50 {
		t.Errorf("Spent = %v, want 50", sum.Spent)
	}
	if sum.Saved != 100 {
		t.Errorf("Saved = %v, want 100", sum.Saved)
	}
	if sum.Income != 200 {
		t.Errorf("Income = %v, want 200", sum.Income)
	}
	if len(sum.Categories) != 2 {
		t.Fatalf("spending categories = %d, want 2", len(sum.Categories))
	}
	for _, c := range sum.Categories {
		if c.Category.Savings {
			t.Errorf("savings category %q must not appear in spending breakdown", c.Category.Name)
		}
	}
	// Sorted by spend descending: Groceries (30) before Uncategorized (20).
	if sum.Categories[0].Category.Name != "Groceries" {
		t.Errorf("top category = %q, want Groceries", sum.Categories[0].Category.Name)
	}
}
