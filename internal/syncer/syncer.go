// Package syncer scarica le transazioni dei conti autorizzati e le salva in
// SQLite in modo incrementale e idempotente.
package syncer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"expense_monitor/internal/config"
	"expense_monitor/internal/enablebanking"
	"expense_monitor/internal/session"
	"expense_monitor/internal/store"
)

// Result riassume l'esito di una sincronizzazione.
type Result struct {
	Accounts        int
	NewTransactions int
}

// RunOnce apre le dipendenze da config ed esegue una singola sincronizzazione.
func RunOnce(ctx context.Context, cfg *config.Config) error {
	client, err := enablebanking.New(cfg.EnableBanking.BaseURL, cfg.EnableBanking.ApplicationID, cfg.EnableBanking.PrivateKeyPath)
	if err != nil {
		return err
	}
	sess, err := session.Load(cfg.Storage.SessionPath)
	if err != nil {
		return fmt.Errorf("sessione non disponibile (%w): esegui prima `auth`", err)
	}
	st, err := store.Open(cfg.Storage.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	res, err := Sync(ctx, cfg, client, sess, st)
	if err != nil {
		return err
	}
	fmt.Printf("Sincronizzazione completata: %d conti, %d nuove transazioni.\n", res.Accounts, res.NewTransactions)
	return nil
}

// Sync sincronizza tutti i conti della sessione. Ritorna ErrSessionExpired se
// il consenso è scaduto (localmente o via 401/403 dall'API).
func Sync(ctx context.Context, cfg *config.Config, client *enablebanking.Client, sess *session.Session, st *store.Store) (Result, error) {
	var res Result
	if sess.Expired() {
		return res, fmt.Errorf("%w: valid_until=%s", enablebanking.ErrSessionExpired, sess.ValidUntil.Format(time.RFC3339))
	}

	for _, acc := range sess.Accounts {
		if err := st.UpsertAccount(store.AccountRecord{
			UID:      acc.UID,
			IBAN:     acc.IBAN,
			Name:     acc.Name,
			Currency: acc.Currency,
		}); err != nil {
			return res, err
		}

		newTx, lastBooking, err := syncAccount(ctx, cfg, client, st, acc.UID)
		if err != nil {
			return res, err
		}
		res.Accounts++
		res.NewTransactions += newTx

		// Balances are best-effort: a failure here must not abort the sync.
		syncBalances(ctx, client, st, acc.UID)

		if err := st.SetSyncState(store.SyncState{
			AccountUID:      acc.UID,
			LastSyncedAt:    time.Now().UTC().Format(time.RFC3339),
			LastBookingDate: lastBooking,
		}); err != nil {
			return res, err
		}
	}
	return res, nil
}

// syncBalances fetches the current balances of an account and stores them.
// Errors are ignored on purpose (balances are secondary to transactions).
func syncBalances(ctx context.Context, client *enablebanking.Client, st *store.Store, accountUID string) {
	balances, err := client.GetBalances(ctx, accountUID)
	if err != nil {
		return
	}
	for _, b := range balances {
		amount, _ := strconv.ParseFloat(strings.TrimSpace(b.BalanceAmount.Amount), 64)
		_ = st.UpsertBalance(store.BalanceRecord{
			AccountUID:    accountUID,
			BalanceType:   b.BalanceType,
			Amount:        amount,
			Currency:      b.BalanceAmount.Currency,
			ReferenceDate: b.ReferenceDate,
		})
	}
}

// syncAccount pagina tutte le transazioni di un conto a partire da date_from
// (incrementale) e le salva.
func syncAccount(ctx context.Context, cfg *config.Config, client *enablebanking.Client, st *store.Store, accountUID string) (int, string, error) {
	prev, err := st.GetSyncState(accountUID)
	if err != nil {
		return 0, "", err
	}

	dateFrom := prev.LastBookingDate
	if dateFrom == "" {
		dateFrom = time.Now().AddDate(0, 0, -cfg.Sync.InitialLookbackDays).Format("2006-01-02")
	}
	lastBooking := prev.LastBookingDate

	newCount := 0
	continuation := ""
	for {
		page, err := client.GetTransactionsPage(ctx, accountUID, enablebanking.TransactionsParams{
			DateFrom:        dateFrom,
			ContinuationKey: continuation,
		})
		if err != nil {
			return newCount, lastBooking, err
		}

		records := make([]store.TxRecord, 0, len(page.Transactions))
		for _, rt := range page.Transactions {
			rec := toRecord(accountUID, rt)
			records = append(records, rec)
			if rec.BookingDate > lastBooking {
				lastBooking = rec.BookingDate
			}
		}
		n, err := st.UpsertTransactions(records)
		if err != nil {
			return newCount, lastBooking, err
		}
		newCount += n

		if page.ContinuationKey == "" {
			break
		}
		continuation = page.ContinuationKey
	}
	return newCount, lastBooking, nil
}

// toRecord mappa una transazione dell'API alla riga di SQLite.
func toRecord(accountUID string, rt enablebanking.RawTransaction) store.TxRecord {
	tx := rt.Tx
	amount, _ := strconv.ParseFloat(strings.TrimSpace(tx.TransactionAmount.Amount), 64)
	if strings.EqualFold(tx.CreditDebitIndicator, "DBIT") {
		amount = -amount
	}
	remittance := strings.Join(tx.RemittanceInformation, "\n")

	return store.TxRecord{
		AccountUID:           accountUID,
		DedupKey:             dedupKey(tx),
		TransactionID:        tx.TransactionID,
		Amount:               amount,
		Currency:             tx.TransactionAmount.Currency,
		CreditDebitIndicator: tx.CreditDebitIndicator,
		Status:               tx.Status,
		BookingDate:          tx.BookingDate,
		ValueDate:            tx.ValueDate,
		TransactionDate:      tx.TransactionDate,
		Reference:            firstNonEmpty(tx.ReferenceNumber, tx.EntryReference),
		Remittance:           remittance,
		CreditorName:         tx.Creditor.Name,
		DebtorName:           tx.Debtor.Name,
		RawJSON:              string(rt.Raw),
	}
}

// dedupKey usa il transaction_id se presente, altrimenti un hash stabile dei
// campi identificativi (alcune banche non forniscono transaction_id).
func dedupKey(tx enablebanking.Transaction) string {
	if tx.TransactionID != "" {
		return "id:" + tx.TransactionID
	}
	if tx.EntryReference != "" {
		return "ref:" + tx.EntryReference
	}
	h := sha256.Sum256([]byte(strings.Join([]string{
		tx.BookingDate,
		tx.ValueDate,
		tx.TransactionAmount.Amount,
		tx.TransactionAmount.Currency,
		tx.CreditDebitIndicator,
		strings.Join(tx.RemittanceInformation, "|"),
	}, "\x1f")))
	return "h:" + hex.EncodeToString(h[:])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
