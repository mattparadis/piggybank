// Package category classifies transactions into user-defined categories based
// on keyword rules from the YAML config, and exposes monthly budgets.
package category

import (
	"strings"

	"expense_monitor/internal/config"
)

// Uncategorized is returned when no rule matches.
var Uncategorized = Category{Name: "Uncategorized", Color: "#9e9e9e", Icon: "❓"}

// Category is a resolved category with its display attributes.
type Category struct {
	Name  string
	Color string
	Icon  string
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
		r := rule{cat: Category{Name: cat.Name, Color: cat.Color, Icon: cat.Icon}}
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
