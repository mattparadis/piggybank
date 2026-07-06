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
func (r *Resolver) Summarize(txs []store.TxRecord) Summary {
	var sum Summary
	byCat := map[string]*CatSpend{}
	var order []string
	for _, t := range txs {
		if t.Amount >= 0 {
			sum.Income += t.Amount
			continue
		}
		amt := -t.Amount
		cat := r.Resolve(t)
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

// ByName resolves a category name to its full display attributes (color, icon,
// savings) from the configured rules. Unknown names and "Uncategorized" map to
// the Uncategorized category.
func (c *Categorizer) ByName(name string) Category {
	if name == "" || name == Uncategorized.Name {
		return Uncategorized
	}
	for _, r := range c.rules {
		if r.cat.Name == name {
			return r.cat
		}
	}
	return Uncategorized
}

// LearnedRule is a user-created keyword -> category mapping (keyword is a
// case-insensitive substring, matched like the configured rules).
type LearnedRule struct {
	Keyword  string
	Category string
}

// Resolver resolves a transaction's category using, in priority order: a manual
// per-transaction override, then learned rules, then the configured keyword
// rules. It layers per-request dynamic data on top of the static Categorizer.
type Resolver struct {
	base      *Categorizer
	overrides map[int64]string
	learned   []LearnedRule // keyword lower-cased
}

// NewResolver builds a Resolver from per-request override and learned-rule data.
// Either argument may be nil/empty, in which case it behaves like the base
// Categorizer.
func (c *Categorizer) NewResolver(overrides map[int64]string, learned []LearnedRule) *Resolver {
	norm := make([]LearnedRule, 0, len(learned))
	for _, lr := range learned {
		kw := strings.ToLower(strings.TrimSpace(lr.Keyword))
		if kw != "" {
			norm = append(norm, LearnedRule{Keyword: kw, Category: lr.Category})
		}
	}
	return &Resolver{base: c, overrides: overrides, learned: norm}
}

// Resolve returns the effective category for a transaction.
func (r *Resolver) Resolve(tx store.TxRecord) Category {
	if r.overrides != nil {
		if name, ok := r.overrides[tx.ID]; ok {
			return r.base.ByName(name)
		}
	}
	if len(r.learned) > 0 {
		text := strings.ToLower(strings.Join([]string{tx.Remittance, tx.CreditorName, tx.DebtorName, tx.Reference}, " \x1f "))
		for _, lr := range r.learned {
			if strings.Contains(text, lr.Keyword) {
				return r.base.ByName(lr.Category)
			}
		}
	}
	return r.base.Categorize(tx.Remittance, tx.CreditorName, tx.DebtorName, tx.Reference)
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
