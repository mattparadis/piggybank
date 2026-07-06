// Package server implements the `serve` command: a daemon that syncs
// transactions periodically, serves the web dashboard and pushes Telegram
// notifications, all sharing the same process and store.
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
	"expense_monitor/internal/telegram"
)

// Run starts the daemon: one sync on startup, then at regular intervals until
// the context is canceled (SIGINT/SIGTERM).
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

	var mon *telegram.Monitor
	if cfg.Telegram.Enabled {
		cat := category.Build(cfg.Dashboard.Categories, cfg.Dashboard.Budgets)
		mon = telegram.NewMonitor(cfg.Telegram, st, cat)
		mon.StartupPing(ctx)
		// Listen for bot commands (/report, /spending, /help) in the background.
		go mon.Listen(ctx)
	}

	n.Info(fmt.Sprintf("daemon started, syncing every %s", interval))
	runSync(ctx, cfg, client, st, n, mon)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			n.Info("daemon stopping")
			return nil
		case <-ticker.C:
			runSync(ctx, cfg, client, st, n, mon)
		}
	}
}

// runSync reloads the session (it may have been refreshed by `auth`) and runs a
// sync, reporting expiry without terminating the daemon. Telegram notifications
// are pushed via mon when configured.
func runSync(ctx context.Context, cfg *config.Config, client *enablebanking.Client, st *store.Store, n notify.Notifier, mon *telegram.Monitor) {
	sess, err := session.Load(cfg.Storage.SessionPath)
	if err != nil {
		n.Alert(fmt.Sprintf("session unavailable (%v): run `auth`", err))
		return
	}

	res, err := syncer.Sync(ctx, cfg, client, sess, st)
	if err != nil {
		if errors.Is(err, enablebanking.ErrSessionExpired) {
			n.Alert("session expired or invalid: run `auth` again to re-authorize")
			if mon != nil {
				mon.SessionExpired(ctx)
			}
			return
		}
		n.Alert(fmt.Sprintf("sync failed: %v", err))
		if mon != nil {
			mon.SyncFailed(ctx, err)
		}
		return
	}
	n.Info(fmt.Sprintf("sync ok: %d accounts, %d new transactions", res.Accounts, res.NewTransactions))
	if mon != nil {
		mon.AfterSync(ctx)
	}
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
		n.Info("dashboard listening on https://" + cfg.AuthServer.ListenAddr)
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
		return fmt.Errorf("dashboard.enabled is false: enable it in the config")
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
