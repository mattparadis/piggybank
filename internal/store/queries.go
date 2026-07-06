package store

import (
	"database/sql"
	"strings"
)

// BalanceRecord is a row of the balances table.
type BalanceRecord struct {
	AccountUID    string
	BalanceType   string
	Amount        float64
	Currency      string
	ReferenceDate string
}

// TxFilter selects and paginates transactions for the dashboard.
type TxFilter struct {
	AccountUID string // empty = all accounts
	DateFrom   string // YYYY-MM-DD inclusive, empty = no lower bound
	DateTo     string // YYYY-MM-DD inclusive, empty = no upper bound
	Text       string // case-insensitive match on remittance/counterparty/reference
	Direction  string // "CRDT", "DBIT" or empty = both
	Limit      int    // 0 = no limit
	Offset     int
}

// MonthSum aggregates incoming/outgoing amounts for a month (YYYY-MM).
type MonthSum struct {
	Month string
	In    float64
	Out   float64
}

// DaySum aggregates incoming/outgoing amounts for a day (YYYY-MM-DD).
type DaySum struct {
	Day string
	In  float64
	Out float64
}

// UpsertBalance inserts or updates a balance for an account/type.
func (s *Store) UpsertBalance(b BalanceRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO balances (account_uid, balance_type, amount, currency, reference_date, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_uid, balance_type) DO UPDATE SET
			amount=excluded.amount,
			currency=excluded.currency,
			reference_date=excluded.reference_date,
			updated_at=excluded.updated_at`,
		b.AccountUID, b.BalanceType, b.Amount, b.Currency, b.ReferenceDate, nowUTC())
	return err
}

// ListAccounts returns all known accounts, ordered by name.
func (s *Store) ListAccounts() ([]AccountRecord, error) {
	rows, err := s.db.Query(`
		SELECT account_uid, COALESCE(iban,''), COALESCE(name,''),
		       COALESCE(currency,''), COALESCE(product,'')
		FROM accounts ORDER BY name, account_uid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AccountRecord
	for rows.Next() {
		var a AccountRecord
		if err := rows.Scan(&a.UID, &a.IBAN, &a.Name, &a.Currency, &a.Product); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListBalances returns all stored balances.
func (s *Store) ListBalances() ([]BalanceRecord, error) {
	rows, err := s.db.Query(`
		SELECT account_uid, balance_type, amount, COALESCE(currency,''), COALESCE(reference_date,'')
		FROM balances ORDER BY account_uid, balance_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BalanceRecord
	for rows.Next() {
		var b BalanceRecord
		if err := rows.Scan(&b.AccountUID, &b.BalanceType, &b.Amount, &b.Currency, &b.ReferenceDate); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// whereClause builds the shared WHERE conditions and args for a TxFilter.
func (f TxFilter) whereClause() (string, []any) {
	var conds []string
	var args []any
	if f.AccountUID != "" {
		conds = append(conds, "account_uid = ?")
		args = append(args, f.AccountUID)
	}
	if f.DateFrom != "" {
		conds = append(conds, "booking_date >= ?")
		args = append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		conds = append(conds, "booking_date <= ?")
		args = append(args, f.DateTo)
	}
	if f.Direction == "CRDT" || f.Direction == "DBIT" {
		conds = append(conds, "credit_debit_indicator = ?")
		args = append(args, f.Direction)
	}
	if f.Text != "" {
		conds = append(conds, "(lower(remittance_information) LIKE ? OR lower(creditor_name) LIKE ? OR lower(debtor_name) LIKE ? OR lower(reference) LIKE ?)")
		like := "%" + strings.ToLower(f.Text) + "%"
		args = append(args, like, like, like, like)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// ListTransactions returns transactions matching the filter, newest first.
func (s *Store) ListTransactions(f TxFilter) ([]TxRecord, error) {
	where, args := f.whereClause()
	q := txSelectColumns + ` FROM transactions` + where + ` ORDER BY booking_date DESC, id DESC`
	if f.Limit > 0 {
		q += " LIMIT ? OFFSET ?"
		args = append(args, f.Limit, f.Offset)
	}

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TxRecord
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// txSelectColumns is the shared column list for reading TxRecord rows.
const txSelectColumns = `SELECT id, account_uid, COALESCE(transaction_id,''), amount, COALESCE(currency,''),
	COALESCE(credit_debit_indicator,''), COALESCE(status,''),
	COALESCE(booking_date,''), COALESCE(value_date,''), COALESCE(transaction_date,''),
	COALESCE(reference,''), COALESCE(remittance_information,''),
	COALESCE(creditor_name,''), COALESCE(debtor_name,'')`

// scanTx scans one row selected via txSelectColumns.
func scanTx(sc interface{ Scan(...any) error }) (TxRecord, error) {
	var t TxRecord
	err := sc.Scan(
		&t.ID, &t.AccountUID, &t.TransactionID, &t.Amount, &t.Currency, &t.CreditDebitIndicator,
		&t.Status, &t.BookingDate, &t.ValueDate, &t.TransactionDate, &t.Reference,
		&t.Remittance, &t.CreditorName, &t.DebtorName)
	return t, err
}

// GetTransaction returns a single transaction by its row id.
func (s *Store) GetTransaction(id int64) (TxRecord, error) {
	return scanTx(s.db.QueryRow(txSelectColumns+` FROM transactions WHERE id = ?`, id))
}

// GetKV returns the value for a key and whether it was present.
func (s *Store) GetKV(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// SetKV inserts or updates a key/value pair.
func (s *Store) SetKV(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO kv (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// LearnedRuleRecord is a stored user-created category rule.
type LearnedRuleRecord struct {
	ID       int64
	Keyword  string
	Category string
}

// SetCategoryOverride assigns a manual category to a single transaction.
func (s *Store) SetCategoryOverride(txID int64, category string) error {
	_, err := s.db.Exec(`
		INSERT INTO category_overrides (tx_id, category_name) VALUES (?, ?)
		ON CONFLICT(tx_id) DO UPDATE SET category_name=excluded.category_name`, txID, category)
	return err
}

// DeleteCategoryOverride removes the manual category of a transaction (reverting
// it to the rule-based categorization).
func (s *Store) DeleteCategoryOverride(txID int64) error {
	_, err := s.db.Exec(`DELETE FROM category_overrides WHERE tx_id = ?`, txID)
	return err
}

// ListCategoryOverrides returns all manual overrides as tx id -> category name.
func (s *Store) ListCategoryOverrides() (map[int64]string, error) {
	rows, err := s.db.Query(`SELECT tx_id, category_name FROM category_overrides`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int64]string)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// AddLearnedRule stores a keyword -> category rule.
func (s *Store) AddLearnedRule(keyword, category string) error {
	_, err := s.db.Exec(`
		INSERT INTO learned_rules (keyword, category_name, created_at) VALUES (?, ?, ?)`,
		keyword, category, nowUTC())
	return err
}

// DeleteLearnedRule removes a learned rule by id.
func (s *Store) DeleteLearnedRule(id int64) error {
	_, err := s.db.Exec(`DELETE FROM learned_rules WHERE id = ?`, id)
	return err
}

// ListLearnedRules returns learned rules, newest first.
func (s *Store) ListLearnedRules() ([]LearnedRuleRecord, error) {
	rows, err := s.db.Query(`SELECT id, keyword, category_name FROM learned_rules ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LearnedRuleRecord
	for rows.Next() {
		var r LearnedRuleRecord
		if err := rows.Scan(&r.ID, &r.Keyword, &r.Category); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListMonth returns all transactions booked in the given month (YYYY-MM),
// used to compute savings-aware spending summaries.
func (s *Store) ListMonth(month string) ([]TxRecord, error) {
	return s.ListTransactions(TxFilter{DateFrom: month + "-01", DateTo: month + "-31"})
}

// CountTransactions returns the number of transactions matching the filter
// (ignoring Limit/Offset), for pagination.
func (s *Store) CountTransactions(f TxFilter) (int, error) {
	where, args := f.whereClause()
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM transactions`+where, args...).Scan(&n)
	return n, err
}

// SumByMonth aggregates credits and debits per month (YYYY-MM) over a date range.
func (s *Store) SumByMonth(from, to string) ([]MonthSum, error) {
	return s.sumByPeriod(7, from, to)
}

// SumByDay aggregates credits and debits per day (YYYY-MM-DD) over a date range.
func (s *Store) SumByDay(from, to string) ([]DaySum, error) {
	res, err := s.sumByPeriod(10, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]DaySum, len(res))
	for i, r := range res {
		out[i] = DaySum{Day: r.Month, In: r.In, Out: r.Out}
	}
	return out, nil
}

// sumByPeriod groups by the first `keyLen` chars of booking_date (7 = month, 10 = day).
func (s *Store) sumByPeriod(keyLen int, from, to string) ([]MonthSum, error) {
	rows, err := s.db.Query(`
		SELECT substr(booking_date, 1, ?) AS period,
		       COALESCE(SUM(CASE WHEN amount > 0 THEN amount ELSE 0 END), 0)  AS inflow,
		       COALESCE(SUM(CASE WHEN amount < 0 THEN -amount ELSE 0 END), 0) AS outflow
		FROM transactions
		WHERE booking_date >= ? AND booking_date <= ?
		GROUP BY period
		ORDER BY period`,
		keyLen, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MonthSum
	for rows.Next() {
		var m MonthSum
		if err := rows.Scan(&m.Month, &m.In, &m.Out); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
