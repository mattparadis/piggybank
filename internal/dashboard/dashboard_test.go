package dashboard

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"expense_monitor/internal/category"
	"expense_monitor/internal/config"
	"expense_monitor/internal/store"
)

func TestBasicAuth(t *testing.T) {
	cfg := &config.Config{}
	cfg.Dashboard.BasicAuth = config.BasicAuth{Username: "u", Password: "p"}
	s := &Server{cfg: cfg}
	protected := s.basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name       string
		setCreds   bool
		user, pass string
		want       int
	}{
		{"no creds", false, "", "", http.StatusUnauthorized},
		{"wrong pass", true, "u", "nope", http.StatusUnauthorized},
		{"wrong user", true, "x", "p", http.StatusUnauthorized},
		{"correct", true, "u", "p", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.setCreds {
				req.SetBasicAuth(tc.user, tc.pass)
			}
			rec := httptest.NewRecorder()
			protected.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

// TestOverviewRenders exercises the full handler + template set to catch
// template errors, with a seeded store.
func TestOverviewRenders(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.UpsertAccount(store.AccountRecord{UID: "a1", IBAN: "IT60X0000", Name: "Main", Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertTransactions([]store.TxRecord{
		{AccountUID: "a1", DedupKey: "1", Amount: -50, Currency: "EUR", CreditDebitIndicator: "DBIT", BookingDate: "2026-07-02", Remittance: "ESSELUNGA"},
		{AccountUID: "a1", DedupKey: "2", Amount: 1500, Currency: "EUR", CreditDebitIndicator: "CRDT", BookingDate: "2026-07-01", Remittance: "SALARY"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertBalance(store.BalanceRecord{AccountUID: "a1", BalanceType: "CLBD", Amount: 1450, Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Dashboard.BasicAuth = config.BasicAuth{Username: "u", Password: "p"}
	cat := category.Build([]config.Category{{Name: "Groceries", Color: "#0f0", Icon: "🛒", MatchAny: []string{"ESSELUNGA"}}}, nil)

	h, err := New(cfg, st, cat)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, path := range []string{"/", "/transactions", "/transactions/rows"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.SetBasicAuth("u", "p")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d, body: %s", path, rec.Code, rec.Body.String())
		}
	}

	// spot-check overview content
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("u", "p")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{"Main", "Groceries", "Overview"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview body missing %q", want)
		}
	}
}
