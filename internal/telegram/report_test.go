package telegram

import (
	"strings"
	"testing"

	"expense_monitor/internal/category"
)

func sampleSummary() category.Summary {
	return category.Summary{
		Spent:  150,
		Saved:  100,
		Income: 400,
		Categories: []category.CatSpend{
			{Category: category.Category{Name: "Groceries", Color: "#4caf50", Icon: "🛒"}, Spent: 100},
			{Category: category.Category{Name: "Dining", Color: "#ff9800", Icon: "🍽"}, Spent: 50},
		},
	}
}

func TestFormatReport(t *testing.T) {
	text := formatReport("2026-07", sampleSummary())
	for _, want := range []string{"2026-07", "Spent", "Saved", "Income", "Groceries", "Dining"} {
		if !strings.Contains(text, want) {
			t.Errorf("report missing %q:\n%s", want, text)
		}
	}
}

func TestRenderCategoryChartPNG(t *testing.T) {
	png, err := renderCategoryChart("2026-07", sampleSummary())
	if err != nil {
		t.Fatalf("renderCategoryChart: %v", err)
	}
	// PNG magic number: 0x89 'P' 'N' 'G'.
	if len(png) < 8 || png[0] != 0x89 || string(png[1:4]) != "PNG" {
		t.Fatalf("output is not a PNG (len %d)", len(png))
	}
}

func TestRenderCategoryChartEmpty(t *testing.T) {
	if _, err := renderCategoryChart("2026-07", category.Summary{}); err == nil {
		t.Fatal("expected error when there are no categories")
	}
}
