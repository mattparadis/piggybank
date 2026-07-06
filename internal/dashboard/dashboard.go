// Package dashboard serves the expense monitor web UI (overview, charts,
// category breakdown and a searchable transaction list) over HTTP. It is
// mounted by the `serve` daemon behind HTTPS + basic auth.
package dashboard

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"expense_monitor/internal/category"
	"expense_monitor/internal/config"
	"expense_monitor/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

const pageSize = 50

// Server renders the dashboard from the store and categorizer.
type Server struct {
	cfg  *config.Config
	st   *store.Store
	cat  *category.Categorizer
	tmpl *template.Template
	now  func() time.Time
}

// New builds the dashboard HTTP handler (wrapped in basic auth).
func New(cfg *config.Config, st *store.Store, cat *category.Categorizer) (http.Handler, error) {
	tmpl, err := template.New("").Funcs(funcMap()).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	s := &Server{cfg: cfg, st: st, cat: cat, tmpl: tmpl, now: time.Now}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleOverview)
	mux.HandleFunc("GET /transactions", s.handleTransactions)
	mux.HandleFunc("GET /transactions/rows", s.handleTransactionRows)
	mux.HandleFunc("POST /transactions/category", s.handleSetCategory)
	mux.HandleFunc("GET /transactions/rule-form", s.handleRuleForm)
	mux.HandleFunc("GET /rules", s.handleRulesList)
	mux.HandleFunc("POST /rules", s.handleAddRule)
	mux.HandleFunc("POST /rules/delete", s.handleDeleteRule)

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	return s.basicAuth(mux), nil
}

func (s *Server) basicAuth(next http.Handler) http.Handler {
	want := s.cfg.Dashboard.BasicAuth
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok ||
			subtle.ConstantTimeCompare([]byte(user), []byte(want.Username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(pass), []byte(want.Password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="expense_monitor"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// ---- Overview ----

type accountBalanceVM struct {
	Name     string
	IBANTail string
	Currency string
	Amount   float64
}

type categorySpendVM struct {
	Category  category.Category
	Spent     float64
	Budget    float64
	HasBudget bool
	Pct       int // spent/budget, capped at 100 for the bar width
	Over      bool
}

type txVM struct {
	Tx       store.TxRecord
	Category category.Category
}

type monthlyChart struct {
	Labels []string  `json:"labels"`
	In     []float64 `json:"in"`
	Out    []float64 `json:"out"`
}

type catChart struct {
	Labels []string  `json:"labels"`
	Values []float64 `json:"values"`
	Colors []string  `json:"colors"`
}

type overviewVM struct {
	Title         string
	Nav           string
	Accounts      []accountBalanceVM
	MonthLabel    string
	MonthIn       float64
	MonthOut      float64
	MonthSaved    float64
	Currency      string
	CategorySpend []categorySpendVM
	Recent        []txVM
	MonthlyChart  monthlyChart
	CategoryChart catChart
	Categories    []category.Category
	LearnedRules  []store.LearnedRuleRecord
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	now := s.now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	from := monthStart.Format("2006-01-02")
	to := now.Format("2006-01-02")

	vm := overviewVM{Title: "Overview", Nav: "overview", MonthLabel: monthStart.Format("January 2006")}
	vm.Categories = s.cat.Categories()

	res, err := s.resolver()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if vm.LearnedRules, err = s.st.ListLearnedRules(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Balances per account.
	accounts, err := s.st.ListAccounts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	balances, err := s.st.ListBalances()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	primary := pickPrimaryBalances(balances)
	for _, a := range accounts {
		vm.Accounts = append(vm.Accounts, accountBalanceVM{
			Name:     accountLabel(a),
			IBANTail: ibanTail(a.IBAN),
			Currency: primary[a.UID].Currency,
			Amount:   primary[a.UID].Amount,
		})
	}
	if len(accounts) > 0 {
		vm.Currency = accounts[0].Currency
	}

	// Current-month transactions for totals and category breakdown.
	monthTx, err := s.st.ListTransactions(store.TxFilter{DateFrom: from, DateTo: to})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Savings-aware totals: spending excludes savings categories, which are
	// reported separately as MonthSaved.
	sum := res.Summarize(monthTx)
	vm.MonthIn = sum.Income
	vm.MonthOut = sum.Spent
	vm.MonthSaved = sum.Saved
	for _, cs := range sum.Categories {
		csvm := categorySpendVM{Category: cs.Category, Spent: cs.Spent}
		if limit, ok := s.cat.Budget(cs.Category.Name); ok {
			csvm.HasBudget = true
			csvm.Budget = limit
			if limit > 0 {
				csvm.Pct = int(math.Min(100, cs.Spent/limit*100))
				csvm.Over = cs.Spent > limit
			}
		}
		vm.CategorySpend = append(vm.CategorySpend, csvm)
	}

	// Category doughnut chart (current month).
	for _, cs := range vm.CategorySpend {
		vm.CategoryChart.Labels = append(vm.CategoryChart.Labels, cs.Category.Name)
		vm.CategoryChart.Values = append(vm.CategoryChart.Values, round2(cs.Spent))
		vm.CategoryChart.Colors = append(vm.CategoryChart.Colors, colorOr(cs.Category.Color))
	}

	// Monthly in/out chart for the last 12 months.
	start12 := monthStart.AddDate(0, -11, 0).Format("2006-01-02")
	months, err := s.st.SumByMonth(start12, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, m := range months {
		vm.MonthlyChart.Labels = append(vm.MonthlyChart.Labels, m.Month)
		vm.MonthlyChart.In = append(vm.MonthlyChart.In, round2(m.In))
		vm.MonthlyChart.Out = append(vm.MonthlyChart.Out, round2(m.Out))
	}

	// Recent transactions.
	recent, err := s.st.ListTransactions(store.TxFilter{Limit: 10})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, t := range recent {
		vm.Recent = append(vm.Recent, txVM{Tx: t, Category: res.Resolve(t)})
	}

	s.render(w, "overview", vm)
}

// ---- Transactions ----

type txListVM struct {
	Title    string
	Nav      string
	Accounts []store.AccountRecord
	Filter   store.TxFilter
	Results  txResultsVM
}

type txResultsVM struct {
	Rows       []txVM
	Categories []category.Category
	Total      int
	Page       int
	Pages      int
	HasPrev    bool
	HasNext    bool
	PrevQuery  template.URL
	NextQuery  template.URL
	RangeStart int
	RangeEnd   int
}

func (s *Server) handleTransactions(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.st.ListAccounts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filter, page := parseFilter(r)
	results, err := s.buildResults(filter, page, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "transactions", txListVM{
		Title:    "Transactions",
		Nav:      "transactions",
		Accounts: accounts,
		Filter:   filter,
		Results:  results,
	})
}

func (s *Server) handleTransactionRows(w http.ResponseWriter, r *http.Request) {
	filter, page := parseFilter(r)
	results, err := s.buildResults(filter, page, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Keep the address bar on the full-page equivalent of this partial, so a
	// reload or shared link reproduces the current filters and page.
	q := filterQuery(r)
	if results.Page > 1 {
		q.Set("page", strconv.Itoa(results.Page))
	}
	target := "/transactions"
	if enc := q.Encode(); enc != "" {
		target += "?" + enc
	}
	w.Header().Set("HX-Replace-Url", target)
	s.render(w, "txresults", results)
}

func (s *Server) buildResults(filter store.TxFilter, page int, r *http.Request) (txResultsVM, error) {
	total, err := s.st.CountTransactions(filter)
	if err != nil {
		return txResultsVM{}, err
	}
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages == 0 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	filter.Limit = pageSize
	filter.Offset = (page - 1) * pageSize

	rows, err := s.st.ListTransactions(filter)
	if err != nil {
		return txResultsVM{}, err
	}
	resolver, err := s.resolver()
	if err != nil {
		return txResultsVM{}, err
	}
	res := txResultsVM{
		Total:      total,
		Categories: s.cat.Categories(),
		Page:       page,
		Pages:      pages,
		HasPrev:    page > 1,
		HasNext:    page < pages,
		RangeStart: min(total, filter.Offset+1),
		RangeEnd:   min(total, filter.Offset+len(rows)),
	}
	for _, t := range rows {
		res.Rows = append(res.Rows, txVM{Tx: t, Category: resolver.Resolve(t)})
	}
	base := filterQuery(r)
	res.PrevQuery = template.URL(withPage(base, page-1))
	res.NextQuery = template.URL(withPage(base, page+1))
	return res, nil
}

// ---- category editing ----

// resolver builds a per-request Resolver from the stored overrides and learned
// rules layered on top of the configured (YAML) categorizer.
func (s *Server) resolver() (*category.Resolver, error) {
	overrides, err := s.st.ListCategoryOverrides()
	if err != nil {
		return nil, err
	}
	rules, err := s.st.ListLearnedRules()
	if err != nil {
		return nil, err
	}
	learned := make([]category.LearnedRule, len(rules))
	for i, r := range rules {
		learned[i] = category.LearnedRule{Keyword: r.Keyword, Category: r.Category}
	}
	return s.cat.NewResolver(overrides, learned), nil
}

// renderCatselect re-renders one transaction's category cell from the current
// overrides and rules, so the chip reflects the freshly resolved category.
func (s *Server) renderCatselect(w http.ResponseWriter, tx store.TxRecord) {
	res, err := s.resolver()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "catselect", map[string]any{
		"TxID":       tx.ID,
		"Current":    res.Resolve(tx),
		"Categories": s.cat.Categories(),
	})
}

// handleSetCategory sets or clears a manual category override for one
// transaction and responds with the re-rendered category cell.
func (s *Server) handleSetCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.FormValue("id"))
	if err != nil {
		http.Error(w, "invalid transaction id", http.StatusBadRequest)
		return
	}
	tx, err := s.st.GetTransaction(id)
	if err != nil {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}
	cat := r.FormValue("category")
	if cat == "__auto__" {
		err = s.st.DeleteCategoryOverride(id)
	} else if !s.validCategory(cat) {
		http.Error(w, "unknown category: "+cat, http.StatusBadRequest)
		return
	} else {
		err = s.st.SetCategoryOverride(id, cat)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderCatselect(w, tx)
}

// handleRuleForm returns the inline "apply to all similar" form, with a keyword
// pre-filled from the transaction.
func (s *Server) handleRuleForm(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "invalid transaction id", http.StatusBadRequest)
		return
	}
	tx, err := s.st.GetTransaction(id)
	if err != nil {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}
	s.render(w, "rule-form", map[string]any{
		"TxID":       id,
		"Keyword":    guessMerchant(tx),
		"Categories": s.cat.Categories(),
	})
}

// handleAddRule stores a learned keyword -> category rule and responds with the
// re-rendered category cell of the transaction the rule was created from.
func (s *Server) handleAddRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.FormValue("id"))
	if err != nil {
		http.Error(w, "invalid transaction id", http.StatusBadRequest)
		return
	}
	tx, err := s.st.GetTransaction(id)
	if err != nil {
		http.Error(w, "transaction not found", http.StatusNotFound)
		return
	}
	keyword := strings.TrimSpace(r.FormValue("keyword"))
	if keyword == "" {
		http.Error(w, "keyword required", http.StatusBadRequest)
		return
	}
	cat := r.FormValue("category")
	if !s.validCategory(cat) {
		http.Error(w, "unknown category: "+cat, http.StatusBadRequest)
		return
	}
	if err := s.st.AddLearnedRule(keyword, cat); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hxTrigger(w, "Rule added", "ok", "rules-changed")
	s.renderCatselect(w, tx)
}

// handleDeleteRule removes a learned rule and responds with the re-rendered
// rules panel.
func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.FormValue("id"))
	if err != nil {
		http.Error(w, "invalid rule id", http.StatusBadRequest)
		return
	}
	if err := s.st.DeleteLearnedRule(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hxTrigger(w, "Rule deleted", "ok")
	s.handleRulesList(w, r)
}

// handleRulesList renders the learned-rules panel partial.
func (s *Server) handleRulesList(w http.ResponseWriter, r *http.Request) {
	rules, err := s.st.ListLearnedRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "rules-list", rules)
}

// guessMerchant returns a best-effort keyword for a new learned rule.
func guessMerchant(tx store.TxRecord) string {
	if tx.CreditorName != "" {
		return tx.CreditorName
	}
	if tx.DebtorName != "" {
		return tx.DebtorName
	}
	rem := tx.Remittance
	if i := strings.Index(strings.ToUpper(rem), "C/O "); i >= 0 {
		if tail := strings.TrimSpace(rem[i+4:]); tail != "" {
			return firstWords(tail, 3)
		}
	}
	if rem != "" {
		return firstWords(rem, 3)
	}
	return tx.Reference
}

func firstWords(s string, n int) string {
	f := strings.Fields(s)
	if len(f) > n {
		f = f[:n]
	}
	return strings.Join(f, " ")
}

// ---- helpers ----

// parseID parses a positive decimal id from a form/query value.
func parseID(v string) (int64, error) {
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id %q", v)
	}
	return id, nil
}

// validCategory reports whether name is a configured category or Uncategorized.
func (s *Server) validCategory(name string) bool {
	if name == category.Uncategorized.Name {
		return true
	}
	for _, c := range s.cat.Categories() {
		if c.Name == name {
			return true
		}
	}
	return false
}

// hxTrigger sets the HX-Trigger header so htmx fires a toast event (picked up
// by app.js) plus any extra bare events on the client.
func hxTrigger(w http.ResponseWriter, message, kind string, extra ...string) {
	events := map[string]any{"toast": map[string]string{"message": message, "kind": kind}}
	for _, e := range extra {
		events[e] = true
	}
	if b, err := json.Marshal(events); err == nil {
		w.Header().Set("HX-Trigger", string(b))
	}
}

func parseFilter(r *http.Request) (store.TxFilter, int) {
	q := r.URL.Query()
	f := store.TxFilter{
		AccountUID: q.Get("account"),
		DateFrom:   q.Get("from"),
		DateTo:     q.Get("to"),
		Text:       q.Get("q"),
		Direction:  q.Get("direction"),
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	return f, page
}

// filterQuery returns the filter parameters (without page) as a query string.
func filterQuery(r *http.Request) url.Values {
	q := r.URL.Query()
	out := url.Values{}
	for _, k := range []string{"account", "from", "to", "q", "direction"} {
		if v := q.Get(k); v != "" {
			out.Set(k, v)
		}
	}
	return out
}

func withPage(v url.Values, page int) string {
	c := url.Values{}
	for k, vs := range v {
		c[k] = vs
	}
	c.Set("page", strconv.Itoa(page))
	return "/transactions/rows?" + c.Encode()
}

func pickPrimaryBalances(balances []store.BalanceRecord) map[string]store.BalanceRecord {
	// Preference order for the "current" balance.
	pref := map[string]int{"CLBD": 3, "XPCD": 2, "ITBD": 1}
	out := map[string]store.BalanceRecord{}
	for _, b := range balances {
		cur, ok := out[b.AccountUID]
		if !ok || pref[b.BalanceType] > pref[cur.BalanceType] {
			out[b.AccountUID] = b
		}
	}
	return out
}

func accountLabel(a store.AccountRecord) string {
	if a.Name != "" {
		return a.Name
	}
	if a.Product != "" {
		return a.Product
	}
	if a.IBAN != "" {
		return a.IBAN
	}
	return a.UID
}

func ibanTail(iban string) string {
	if len(iban) <= 4 {
		return iban
	}
	return "…" + iban[len(iban)-4:]
}

func colorOr(c string) string {
	if c == "" {
		return category.Uncategorized.Color
	}
	return c
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func funcMap() template.FuncMap {
	return template.FuncMap{
		"money": func(v float64) string {
			return strconv.FormatFloat(v, 'f', 2, 64)
		},
		"abs": math.Abs,
		"json": func(v any) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("null")
			}
			return template.JS(b)
		},
		"dict": func(pairs ...any) map[string]any {
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				if key, ok := pairs[i].(string); ok {
					m[key] = pairs[i+1]
				}
			}
			return m
		},
	}
}
