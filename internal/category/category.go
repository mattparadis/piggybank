// Package category classifies transactions into user-defined categories based
// on keyword rules from the YAML config, and exposes monthly budgets.
package category

import (
	"sort"
	"strings"

	"expense_monitor/internal/config"
	"expense_monitor/internal/store"
)

// Uncategorized is returned when no rule matches.
var Uncategorized = Category{Name: "Uncategorized", Color: "#9e9e9e", Icon: "❓"}

// Category is a resolved category with its display attributes. When Savings is
// true the category represents money moved to savings (a transfer), which is
// excluded from spending totals and reported separately as an amount saved.
type Category struct {
	Name    string
	Color   string
	Icon    string
	Savings bool
}

type rule struct {
	cat   Category
	match []string // lower-cased substrings; any match wins
}

// Categorizer applies category rules and holds monthly budgets.
type Categorizer struct {
	rules   []rule
	budgets map[string]float64
}

// Build compiles the categorizer from config categories and budgets.
func Build(cats []config.Category, budgets []config.Budget) *Categorizer {
	c := &Categorizer{budgets: make(map[string]float64, len(budgets))}
	for _, cat := range cats {
		r := rule{cat: Category{Name: cat.Name, Color: cat.Color, Icon: cat.Icon, Savings: cat.Savings}}
		for _, m := range cat.MatchAny {
			if m = strings.ToLower(strings.TrimSpace(m)); m != "" {
				r.match = append(r.match, m)
			}
		}
		c.rules = append(c.rules, r)
	}
	for _, b := range budgets {
		c.budgets[b.Category] = b.MonthlyLimit
	}
	return c
}

// Categorize returns the first category whose keywords appear in any of the
// provided text fields (case-insensitive), or Uncategorized.
func (c *Categorizer) Categorize(fields ...string) Category {
	text := strings.ToLower(strings.Join(fields, " \x1f "))
	for _, r := range c.rules {
		for _, m := range r.match {
			if strings.Contains(text, m) {
				return r.cat
			}
		}
	}
	return Uncategorized
}

// CatSpend is the spending accumulated for one (non-savings) category.
type CatSpend struct {
	Category Category
	Spent    float64
}

// Summary aggregates a set of transactions into spending, savings and income.
// Spending excludes savings categories, which are accumulated in Saved instead.
type Summary struct {
	Spent      float64    // sum of |amount| for debits, EXCLUDING savings
	Saved      float64    // sum of |amount| for debits in savings categories
	Income     float64    // sum of amount for credits
	Categories []CatSpend // non-savings spending, sorted by amount descending
}

// Summarize categorizes the transactions and rolls them up into a Summary.
// Debits (negative amount) count as spending unless their category is a savings
// category, in which case they count as Saved. Credits count as Income.
func (c *Categorizer) Summarize(txs []store.TxRecord) Summary {
	var sum Summary
	byCat := map[string]*CatSpend{}
	var order []string
	for _, t := range txs {
		if t.Amount >= 0 {
			sum.Income += t.Amount
			continue
		}
		amt := -t.Amount
		cat := c.Categorize(t.Remittance, t.CreditorName, t.DebtorName, t.Reference)
		if cat.Savings {
			sum.Saved += amt
			continue
		}
		sum.Spent += amt
		cs := byCat[cat.Name]
		if cs == nil {
			cs = &CatSpend{Category: cat}
			byCat[cat.Name] = cs
			order = append(order, cat.Name)
		}
		cs.Spent += amt
	}
	for _, name := range order {
		sum.Categories = append(sum.Categories, *byCat[name])
	}
	sort.Slice(sum.Categories, func(i, j int) bool {
		return sum.Categories[i].Spent > sum.Categories[j].Spent
	})
	return sum
}

// Budget returns the monthly limit for a category, if configured.
func (c *Categorizer) Budget(category string) (float64, bool) {
	v, ok := c.budgets[category]
	return v, ok
}

// Categories returns the configured categories in order (excluding Uncategorized).
func (c *Categorizer) Categories() []Category {
	out := make([]Category, 0, len(c.rules))
	for _, r := range c.rules {
		out = append(out, r.cat)
	}
	return out
}
