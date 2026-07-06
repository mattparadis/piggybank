package dashboard

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"expense_monitor/internal/category"
	"expense_monitor/internal/config"
	"expense_monitor/internal/store"
)

func TestSetCategoryAndAddRule(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.UpsertAccount(store.AccountRecord{UID: "a1", Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertTransactions([]store.TxRecord{
		{AccountUID: "a1", DedupKey: "1", Amount: -10, CreditDebitIndicator: "DBIT", BookingDate: "2026-07-02", Remittance: "KFC"},
	}); err != nil {
		t.Fatal(err)
	}
	txs, _ := st.ListTransactions(store.TxFilter{})
	id := txs[0].ID

	cfg := &config.Config{}
	cfg.Dashboard.BasicAuth = config.BasicAuth{Username: "u", Password: "p"}
	cat := category.Build([]config.Category{{Name: "Dining", MatchAny: []string{"KFC"}}}, nil)
	h, err := New(cfg, st, cat)
	if err != nil {
		t.Fatal(err)
	}

	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth("u", "p")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Set a manual override.
	rec := post("/transactions/category", url.Values{"id": {fmt.Sprint(id)}, "category": {"Groceries"}})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("set category status = %d", rec.Code)
	}
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Errorf("expected HX-Refresh header")
	}
	ov, _ := st.ListCategoryOverrides()
	if ov[id] != "Groceries" {
		t.Errorf("override = %q, want Groceries", ov[id])
	}

	// Revert to auto deletes the override.
	post("/transactions/category", url.Values{"id": {fmt.Sprint(id)}, "category": {"__auto__"}})
	ov, _ = st.ListCategoryOverrides()
	if _, ok := ov[id]; ok {
		t.Errorf("override should be cleared by __auto__")
	}

	// Add a learned rule.
	rec = post("/rules", url.Values{"keyword": {"KFC"}, "category": {"Dining"}})
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("add rule status = %d, hx-refresh = %q", rec.Code, rec.Header().Get("HX-Refresh"))
	}
	rules, _ := st.ListLearnedRules()
	if len(rules) != 1 || rules[0].Keyword != "KFC" || rules[0].Category != "Dining" {
		t.Fatalf("learned rules = %+v", rules)
	}
}
