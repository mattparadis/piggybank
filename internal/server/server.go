// Package server implementa il comando `serve`: un daemon che sincronizza le
// transazioni periodicamente. Dashboard web e bot Telegram sono predisposti
// (vedi TODO) ma non implementati in questa iterazione.
package server

import (
	"context"
	"errors"
	"fmt"
	"time"

	"expense_monitor/internal/config"
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

	// TODO(dashboard): avviare qui il server HTTP sulla porta configurata
	// (auth_server.listen_addr, 7777) per servire la dashboard delle spese.
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
