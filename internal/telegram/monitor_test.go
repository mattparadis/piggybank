package telegram

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"expense_monitor/internal/category"
	"expense_monitor/internal/config"
	"expense_monitor/internal/store"
)

// fakeSender records messages/photos instead of hitting the Bot API.
type fakeSender struct {
	msgs   []string
	photos []string
}

func (f *fakeSender) SendMessage(_ context.Context, text string) error {
	f.msgs = append(f.msgs, text)
	return nil
}

func (f *fakeSender) SendPhoto(_ context.Context, caption string, _ []byte) error {
	f.photos = append(f.photos, caption)
	return nil
}

func newTestMonitor(t *testing.T, cfg config.TelegramConfig, cats []config.Category) (*Monitor, *fakeSender, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.UpsertAccount(store.AccountRecord{UID: "acc-1", Currency: "EUR"}); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}
	fs := &fakeSender{}
	m := &Monitor{
		cfg:    cfg,
		client: fs,
		st:     st,
		cat:    category.Build(cats, nil),
		now:    time.Now,
	}
	return m, fs, st
}

func addTx(t *testing.T, st *store.Store, key string, amount float64, date, remit string) {
	t.Helper()
	if _, err := st.UpsertTransactions([]store.TxRecord{{
		AccountUID: "acc-1", DedupKey: key, TransactionID: key,
		Amount: amount, Currency: "EUR", Status: "BOOK",
		BookingDate: date, Remittance: remit,
	}}); err != nil {
		t.Fatalf("UpsertTransactions: %v", err)
	}
}

func TestLevelIndex(t *testing.T) {
	cases := []struct {
		spend, threshold, step float64
		want                   int
	}{
		{100, 200, 30, -1}, // below threshold
		{200, 200, 30, 0},  // at threshold
		{229, 200, 30, 0},  // within first step
		{230, 200, 30, 1},  // second step
		{291, 200, 30, 3},  // 200,230,260,290
		{100, 200, 0, -1},  // zero step guarded
	}
	for _, c := range cases {
		if got := levelIndex(c.spend, c.threshold, c.step); got != c.want {
			t.Errorf("levelIndex(%v,%v,%v) = %d, want %d", c.spend, c.threshold, c.step, got, c.want)
		}
	}
}

func TestCheckSpendingBaselineThenSteps(t *testing.T) {
	cfg := config.TelegramConfig{
		SpendingAlert: config.SpendingAlertConfig{Enabled: true, Threshold: 200, Step: 30},
	}
	m, fs, st := newTestMonitor(t, cfg, []config.Category{
		{Name: "Savings", Savings: true, MatchAny: []string{"GIROCONTO"}},
	})
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	ctx := context.Background()

	// Below threshold on first observation -> silent baseline, no message.
	addTx(t, st, "t1", -180, "2026-07-02", "shop")
	addTx(t, st, "s1", -500, "2026-07-02", "GIROCONTO to savings") // excluded from spend
	m.checkSpending(ctx, now)
	if len(fs.msgs) != 0 {
		t.Fatalf("baseline should be silent, got %v", fs.msgs)
	}

	// Cross the threshold (spend 210).
	addTx(t, st, "t2", -30, "2026-07-03", "shop")
	m.checkSpending(ctx, now)
	if len(fs.msgs) != 1 {
		t.Fatalf("crossing threshold should send 1 message, got %d", len(fs.msgs))
	}

	// Cross the next step (spend 245 -> level index 1).
	addTx(t, st, "t3", -35, "2026-07-04", "shop")
	m.checkSpending(ctx, now)
	if len(fs.msgs) != 2 {
		t.Fatalf("crossing next step should send another message, got %d", len(fs.msgs))
	}

	// No further crossing -> no new message.
	m.checkSpending(ctx, now)
	if len(fs.msgs) != 2 {
		t.Fatalf("no crossing should not send a message, got %d", len(fs.msgs))
	}
}

func TestMaybeMonthlyReport(t *testing.T) {
	cfg := config.TelegramConfig{
		MonthlyReport: config.MonthlyReportConfig{Enabled: true, WithChart: false},
	}
	m, fs, st := newTestMonitor(t, cfg, nil)
	ctx := context.Background()

	july := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return july }

	// First run in July: baseline only, no report.
	m.maybeMonthlyReport(ctx, july)
	if len(fs.msgs) != 0 {
		t.Fatalf("first run should not report, got %v", fs.msgs)
	}
	// Same month again: still nothing.
	m.maybeMonthlyReport(ctx, july)
	if len(fs.msgs) != 0 {
		t.Fatalf("same month should not report, got %v", fs.msgs)
	}

	// New month (August): report for the completed month July.
	addTx(t, st, "t1", -50, "2026-07-10", "shop")
	aug := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	m.maybeMonthlyReport(ctx, aug)
	if len(fs.msgs) != 1 {
		t.Fatalf("month rollover should send 1 report, got %d", len(fs.msgs))
	}
	if !strings.Contains(fs.msgs[0], "2026-07") {
		t.Errorf("report should cover July, got %q", fs.msgs[0])
	}
}
