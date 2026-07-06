package telegram

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"expense_monitor/internal/category"
	"expense_monitor/internal/config"
	"expense_monitor/internal/store"
)

// KV keys used to persist notification cursors across syncs and restarts.
const (
	kvSpendMonth  = "tg_spend_month"
	kvSpendIdx    = "tg_spend_idx"
	kvReportMonth = "tg_report_month"
)

// sender is the subset of the Bot API used by the monitor (kept small so it can
// be faked in tests).
type sender interface {
	SendMessage(ctx context.Context, text string) error
	SendPhoto(ctx context.Context, caption string, png []byte) error
}

// Monitor evaluates notification rules against the stored data after each sync
// and pushes Telegram messages accordingly.
type Monitor struct {
	cfg    config.TelegramConfig
	client sender
	st     *store.Store
	cat    *category.Categorizer
	now    func() time.Time
}

// NewMonitor builds a Monitor backed by a real Telegram client.
func NewMonitor(cfg config.TelegramConfig, st *store.Store, cat *category.Categorizer) *Monitor {
	return &Monitor{
		cfg:    cfg,
		client: NewClient(cfg.BotToken, cfg.ChatID),
		st:     st,
		cat:    cat,
		now:    time.Now,
	}
}

// StartupPing sends a one-time confirmation that the bot is configured.
func (m *Monitor) StartupPing(ctx context.Context) {
	if !m.cfg.StartupPing {
		return
	}
	m.send(ctx, "✅ expense_monitor daemon started — Telegram notifications are configured correctly.")
}

// SessionExpired notifies that the Enable Banking consent needs renewing.
func (m *Monitor) SessionExpired(ctx context.Context) {
	if !m.cfg.SessionAlerts {
		return
	}
	m.send(ctx, "🔴 Enable Banking session expired or invalid — run `auth` to re-authorize.")
}

// SyncFailed notifies about a non-expiry sync failure.
func (m *Monitor) SyncFailed(ctx context.Context, cause error) {
	if !m.cfg.SessionAlerts {
		return
	}
	m.send(ctx, "⚠️ Sync failed: "+cause.Error())
}

// AfterSync runs the post-sync checks (spending threshold + monthly report).
func (m *Monitor) AfterSync(ctx context.Context) {
	now := m.now()
	m.checkSpending(ctx, now)
	m.maybeMonthlyReport(ctx, now)
}

// checkSpending alerts when the running monthly spend (savings excluded) crosses
// the configured threshold and each subsequent step. The first observation of a
// month establishes a silent baseline so the initial historical backfill never
// fires an alert.
func (m *Monitor) checkSpending(ctx context.Context, now time.Time) {
	sa := m.cfg.SpendingAlert
	if !sa.Enabled {
		return
	}
	month := now.Format("2006-01")
	txs, err := m.st.ListMonth(month)
	if err != nil {
		log.Printf("[telegram] spending check: %v", err)
		return
	}
	spend := m.cat.Summarize(txs).Spent
	idx := levelIndex(spend, sa.Threshold, sa.Step)

	storedMonth, _, err := m.st.GetKV(kvSpendMonth)
	if err != nil {
		log.Printf("[telegram] spending state: %v", err)
		return
	}
	if storedMonth != month {
		// New month or first-ever run: baseline silently.
		m.setSpendState(month, idx)
		return
	}
	if idx > m.readInt(kvSpendIdx, -1) {
		level := sa.Threshold + float64(idx)*sa.Step
		m.send(ctx, fmt.Sprintf("💸 Monthly spending has reached %.2f — crossed the %.2f mark (%s).", spend, level, month))
		m.setSpendState(month, idx)
	}
}

// maybeMonthlyReport sends a report for the just-completed month the first time a
// sync runs in a new month. The first-ever run only records a baseline.
func (m *Monitor) maybeMonthlyReport(ctx context.Context, now time.Time) {
	if !m.cfg.MonthlyReport.Enabled {
		return
	}
	month := now.Format("2006-01")
	stored, ok, err := m.st.GetKV(kvReportMonth)
	if err != nil {
		log.Printf("[telegram] report state: %v", err)
		return
	}
	if !ok {
		// First run: don't report a possibly-partial previous month.
		_ = m.st.SetKV(kvReportMonth, month)
		return
	}
	if stored == month {
		return
	}
	m.sendMonthlyReport(ctx, stored)
	_ = m.st.SetKV(kvReportMonth, month)
}

func (m *Monitor) sendMonthlyReport(ctx context.Context, month string) {
	txs, err := m.st.ListMonth(month)
	if err != nil {
		log.Printf("[telegram] monthly report: %v", err)
		return
	}
	sum := m.cat.Summarize(txs)
	text := formatReport(month, sum)

	if m.cfg.MonthlyReport.WithChart && len(sum.Categories) > 0 {
		png, err := renderCategoryChart(month, sum)
		if err != nil {
			log.Printf("[telegram] chart render: %v", err)
		} else if err := m.client.SendPhoto(ctx, text, png); err != nil {
			log.Printf("[telegram] send photo: %v", err)
		} else {
			return
		}
	}
	m.send(ctx, text)
}

func (m *Monitor) send(ctx context.Context, text string) {
	if err := m.client.SendMessage(ctx, text); err != nil {
		log.Printf("[telegram] send failed: %v", err)
	}
}

func (m *Monitor) setSpendState(month string, idx int) {
	if err := m.st.SetKV(kvSpendMonth, month); err != nil {
		log.Printf("[telegram] set spend month: %v", err)
	}
	if err := m.st.SetKV(kvSpendIdx, strconv.Itoa(idx)); err != nil {
		log.Printf("[telegram] set spend idx: %v", err)
	}
}

func (m *Monitor) readInt(key string, def int) int {
	v, ok, err := m.st.GetKV(key)
	if err != nil || !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// levelIndex returns -1 when spend is below threshold, else the number of full
// steps above the threshold (0 = threshold, 1 = threshold+step, ...).
func levelIndex(spend, threshold, step float64) int {
	if step <= 0 || spend < threshold {
		return -1
	}
	return int((spend - threshold) / step)
}
