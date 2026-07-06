// Package server implementa il comando `serve`: un daemon che sincronizza le
// transazioni periodicamente. Dashboard web e bot Telegram sono predisposti
// (vedi TODO) ma non implementati in questa iterazione.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"expense_monitor/internal/category"
	"expense_monitor/internal/config"
	"expense_monitor/internal/dashboard"
	"expense_monitor/internal/enablebanking"
	"expense_monitor/internal/notify"
	"expense_monitor/internal/session"
	"expense_monitor/internal/store"
	"expense_monitor/internal/syncer"
)

// Run avvia il daemon: una sync all'avvio, poi a intervalli regolari finché il
// contesto non viene annullato (SIGINT/SIGTERM).
func Run(ctx context.Context, cfg *config.Config) error {
	client, err := enablebanking.New(cfg.EnableBanking.BaseURL, cfg.EnableBanking.ApplicationID, cfg.EnableBanking.PrivateKeyPath)
	if err != nil {
		return err
	}
	st, err := store.Open(cfg.Storage.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	n := notify.LogNotifier{}
	interval := time.Duration(24/cfg.Sync.TimesPerDay) * time.Hour
	if interval <= 0 {
		interval = time.Hour
	}

	if cfg.Dashboard.Enabled {
		stopDashboard, err := startDashboard(cfg, st, n)
		if err != nil {
			return err
		}
		defer stopDashboard()
	}

	// TODO(telegram): avviare qui il bot e usare un notify.TelegramNotifier
	// applicando le regole di cfg.Telegram.Rules.

	n.Info(fmt.Sprintf("daemon avviato, sincronizzazione ogni %s", interval))
	runSync(ctx, cfg, client, st, n)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			n.Info("arresto del daemon")
			return nil
		case <-ticker.C:
			runSync(ctx, cfg, client, st, n)
		}
	}
}

// runSync ricarica la sessione (potrebbe essere stata aggiornata da `auth`) ed
// esegue una sincronizzazione, segnalando la scadenza senza terminare il daemon.
func runSync(ctx context.Context, cfg *config.Config, client *enablebanking.Client, st *store.Store, n notify.Notifier) {
	sess, err := session.Load(cfg.Storage.SessionPath)
	if err != nil {
		n.Alert(fmt.Sprintf("sessione non disponibile (%v): esegui `auth`", err))
		return
	}

	res, err := syncer.Sync(ctx, cfg, client, sess, st)
	if err != nil {
		if errors.Is(err, enablebanking.ErrSessionExpired) {
			n.Alert("sessione scaduta o non valida: rilancia il comando `auth` per riautorizzare")
			return
		}
		n.Alert(fmt.Sprintf("sincronizzazione fallita: %v", err))
		return
	}
	n.Info(fmt.Sprintf("sync ok: %d conti, %d nuove transazioni", res.Accounts, res.NewTransactions))
}

// startDashboard builds the dashboard handler and serves it over HTTPS in a
// goroutine. The returned function shuts the server down gracefully.
func startDashboard(cfg *config.Config, st *store.Store, n notify.Notifier) (func(), error) {
	cat := category.Build(cfg.Dashboard.Categories, cfg.Dashboard.Budgets)
	handler, err := dashboard.New(cfg, st, cat)
	if err != nil {
		return nil, fmt.Errorf("dashboard: %w", err)
	}
	srv := &http.Server{Addr: cfg.AuthServer.ListenAddr, Handler: handler}
	go func() {
		n.Info("dashboard in ascolto su https://" + cfg.AuthServer.ListenAddr)
		err := srv.ListenAndServeTLS(cfg.AuthServer.TLSCertPath, cfg.AuthServer.TLSKeyPath)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			n.Alert("dashboard server: " + err.Error())
		}
	}()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}, nil
}

// RunDashboard serves only the dashboard (no sync), until the context is done.
func RunDashboard(ctx context.Context, cfg *config.Config) error {
	if !cfg.Dashboard.Enabled {
		return fmt.Errorf("dashboard.enabled è false: abilitala nel config")
	}
	st, err := store.Open(cfg.Storage.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	stop, err := startDashboard(cfg, st, notify.LogNotifier{})
	if err != nil {
		return err
	}
	defer stop()

	<-ctx.Done()
	return nil
}
