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

func newEditTestServer(t *testing.T) (http.Handler, *store.Store, int64) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "e.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
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
	cat := category.Build([]config.Category{
		{Name: "Dining", MatchAny: []string{"KFC"}},
		{Name: "Groceries"},
	}, nil)
	h, err := New(cfg, st, cat)
	if err != nil {
		t.Fatal(err)
	}
	return h, st, id
}

func postForm(h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("u", "p")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSetCategoryAndAddRule(t *testing.T) {
	h, st, id := newEditTestServer(t)

	// Set a manual override: responds with the re-rendered catselect fragment.
	rec := postForm(h, "/transactions/category", url.Values{"id": {fmt.Sprint(id)}, "category": {"Groceries"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("set category status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `class="cat-form"`) || !strings.Contains(body, `value="Groceries" selected`) {
		t.Errorf("catselect fragment missing or wrong selection: %s", body)
	}
	ov, _ := st.ListCategoryOverrides()
	if ov[id] != "Groceries" {
		t.Errorf("override = %q, want Groceries", ov[id])
	}

	// Revert to auto deletes the override and resolves back to the rule match.
	rec = postForm(h, "/transactions/category", url.Values{"id": {fmt.Sprint(id)}, "category": {"__auto__"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("auto status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `value="Dining" selected`) {
		t.Errorf("__auto__ should re-resolve to Dining: %s", rec.Body.String())
	}
	ov, _ = st.ListCategoryOverrides()
	if _, ok := ov[id]; ok {
		t.Errorf("override should be cleared by __auto__")
	}

	// Add a learned rule (needs the source transaction id now).
	rec = postForm(h, "/rules", url.Values{"id": {fmt.Sprint(id)}, "keyword": {"KFC"}, "category": {"Dining"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("add rule status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if trig := rec.Header().Get("HX-Trigger"); !strings.Contains(trig, "Rule added") || !strings.Contains(trig, "rules-changed") {
		t.Errorf("HX-Trigger = %q", trig)
	}
	rules, _ := st.ListLearnedRules()
	if len(rules) != 1 || rules[0].Keyword != "KFC" || rules[0].Category != "Dining" {
		t.Fatalf("learned rules = %+v", rules)
	}

	// Delete the rule: responds with the re-rendered rules panel.
	rec = postForm(h, "/rules/delete", url.Values{"id": {fmt.Sprint(rules[0].ID)}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="rules-panel"`) {
		t.Fatalf("delete rule status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if rules, _ = st.ListLearnedRules(); len(rules) != 0 {
		t.Errorf("rules after delete = %+v", rules)
	}
}

func TestEditValidation(t *testing.T) {
	h, st, id := newEditTestServer(t)

	cases := []struct {
		name string
		path string
		form url.Values
		want int
	}{
		{"bad tx id", "/transactions/category", url.Values{"id": {"abc"}, "category": {"Dining"}}, http.StatusBadRequest},
		{"missing tx id", "/transactions/category", url.Values{"category": {"Dining"}}, http.StatusBadRequest},
		{"unknown tx", "/transactions/category", url.Values{"id": {"99999"}, "category": {"Dining"}}, http.StatusNotFound},
		{"unknown category", "/transactions/category", url.Values{"id": {fmt.Sprint(id)}, "category": {"Nope"}}, http.StatusBadRequest},
		{"rule bad id", "/rules", url.Values{"id": {"0"}, "keyword": {"KFC"}, "category": {"Dining"}}, http.StatusBadRequest},
		{"rule empty keyword", "/rules", url.Values{"id": {fmt.Sprint(id)}, "keyword": {"  "}, "category": {"Dining"}}, http.StatusBadRequest},
		{"rule unknown category", "/rules", url.Values{"id": {fmt.Sprint(id)}, "keyword": {"KFC"}, "category": {"Nope"}}, http.StatusBadRequest},
		{"delete bad id", "/rules/delete", url.Values{"id": {"abc"}}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postForm(h, tc.path, tc.form)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d (body: %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}

	// Nothing should have been stored by the rejected requests.
	if ov, _ := st.ListCategoryOverrides(); len(ov) != 0 {
		t.Errorf("overrides = %+v, want none", ov)
	}
	if rules, _ := st.ListLearnedRules(); len(rules) != 0 {
		t.Errorf("rules = %+v, want none", rules)
	}
}

func TestRuleFormAndRowsURL(t *testing.T) {
	h, _, id := newEditTestServer(t)

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.SetBasicAuth("u", "p")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Rule form carries the transaction id and a pre-filled keyword.
	rec := get(fmt.Sprintf("/transactions/rule-form?id=%d", id))
	if rec.Code != http.StatusOK {
		t.Fatalf("rule-form status = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, fmt.Sprintf(`name="id" value="%d"`, id)) || !strings.Contains(body, `value="KFC"`) {
		t.Errorf("rule-form body: %s", body)
	}
	if rec = get("/transactions/rule-form?id=abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("rule-form bad id status = %d", rec.Code)
	}

	// Rows partial rewrites the address bar to the full-page URL.
	rec = get("/transactions/rows?account=a1&q=kfc")
	if got := rec.Header().Get("HX-Replace-Url"); got != "/transactions?account=a1&q=kfc" {
		t.Errorf("HX-Replace-Url = %q", got)
	}
	if rec = get("/transactions/rows"); rec.Header().Get("HX-Replace-Url") != "/transactions" {
		t.Errorf("HX-Replace-Url (no filters) = %q", rec.Header().Get("HX-Replace-Url"))
	}

	// GET /rules renders the panel partial.
	if rec = get("/rules"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `id="rules-panel"`) {
		t.Errorf("GET /rules status = %d, body: %s", rec.Code, rec.Body.String())
	}
}
