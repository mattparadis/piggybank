package category

import (
	"testing"

	"expense_monitor/internal/config"
)

func testCategorizer() *Categorizer {
	return Build(
		[]config.Category{
			{Name: "Groceries", Color: "#0f0", Icon: "🛒", MatchAny: []string{"ESSELUNGA", "coop"}},
			{Name: "Transport", Color: "#00f", MatchAny: []string{"TRENITALIA"}},
		},
		[]config.Budget{{Category: "Groceries", MonthlyLimit: 400}},
	)
}

func TestCategorizeMatchAndCaseInsensitive(t *testing.T) {
	c := testCategorizer()

	if got := c.Categorize("PAGAMENTO ESSELUNGA MILANO", "", "", ""); got.Name != "Groceries" {
		t.Errorf("expected Groceries, got %q", got.Name)
	}
	// case-insensitive: rule "coop" vs text "COOP"
	if got := c.Categorize("", "SUPERMERCATO COOP", "", ""); got.Name != "Groceries" {
		t.Errorf("expected Groceries (case-insensitive), got %q", got.Name)
	}
	// match against a different field (reference)
	if got := c.Categorize("", "", "", "TRENITALIA TICKET"); got.Name != "Transport" {
		t.Errorf("expected Transport, got %q", got.Name)
	}
}

func TestCategorizeFallback(t *testing.T) {
	c := testCategorizer()
	if got := c.Categorize("UNKNOWN MERCHANT", "", "", ""); got.Name != Uncategorized.Name {
		t.Errorf("expected Uncategorized, got %q", got.Name)
	}
}

func TestBudgetLookup(t *testing.T) {
	c := testCategorizer()
	if v, ok := c.Budget("Groceries"); !ok || v != 400 {
		t.Errorf("Budget(Groceries) = %v, %v", v, ok)
	}
	if _, ok := c.Budget("Transport"); ok {
		t.Errorf("Transport should have no budget")
	}
}
