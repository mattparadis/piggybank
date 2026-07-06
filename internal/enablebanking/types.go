package enablebanking

import "encoding/json"

// ASPSP is an available bank (GET /aspsps).
type ASPSP struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}

type aspspsResponse struct {
	ASPSPs []ASPSP `json:"aspsps"`
}

// Access defines until when the consent is valid.
type Access struct {
	ValidUntil string `json:"valid_until"`
}

// ASPSPRef identifies the bank in requests.
type ASPSPRef struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}

// AuthRequest is the body of POST /auth.
type AuthRequest struct {
	Access      Access   `json:"access"`
	ASPSP       ASPSPRef `json:"aspsp"`
	State       string   `json:"state"`
	RedirectURL string   `json:"redirect_url"`
	PSUType     string   `json:"psu_type"`
}

// AuthResponse is the response of POST /auth.
type AuthResponse struct {
	URL             string `json:"url"`
	AuthorizationID string `json:"authorization_id"`
	PSUIDHash       string `json:"psu_id_hash"`
}

// AccountID contains the account IBAN.
type AccountID struct {
	IBAN string `json:"iban"`
}

// Account is an authorized account returned by POST /sessions.
type Account struct {
	UID             string    `json:"uid"`
	AccountID       AccountID `json:"account_id"`
	Name            string    `json:"name"`
	Currency        string    `json:"currency"`
	CashAccountType string    `json:"cash_account_type"`
	Product         string    `json:"product"`
}

// SessionResponse is the response of POST /sessions and GET /sessions/{id}.
type SessionResponse struct {
	SessionID string    `json:"session_id"`
	Accounts  []Account `json:"accounts"`
	ASPSP     ASPSPRef  `json:"aspsp"`
	PSUType   string    `json:"psu_type"`
	Access    Access    `json:"access"`
}

// Amount is a monetary amount (amount is a string in the API).
type Amount struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

// Balance is an account balance (GET /accounts/{uid}/balances). An account may
// expose several balance types (e.g. CLBD closing booked, XPCD expected).
type Balance struct {
	Name               string `json:"name"`
	BalanceAmount      Amount `json:"balance_amount"`
	BalanceType        string `json:"balance_type"`
	ReferenceDate      string `json:"reference_date"`
	LastChangeDateTime string `json:"last_change_date_time"`
}

type balancesResponse struct {
	Balances []Balance `json:"balances"`
}

// Party is a counterparty (creditor/debtor).
type Party struct {
	Name string `json:"name"`
}

// Transaction is a single transaction.
type Transaction struct {
	TransactionID         string    `json:"transaction_id"`
	EntryReference        string    `json:"entry_reference"`
	TransactionAmount     Amount    `json:"transaction_amount"`
	CreditDebitIndicator  string    `json:"credit_debit_indicator"`
	Status                string    `json:"status"`
	BookingDate           string    `json:"booking_date"`
	ValueDate             string    `json:"value_date"`
	TransactionDate       string    `json:"transaction_date"`
	RemittanceInformation []string  `json:"remittance_information"`
	Creditor              Party     `json:"creditor"`
	CreditorAccount       AccountID `json:"creditor_account"`
	Debtor                Party     `json:"debtor"`
	DebtorAccount         AccountID `json:"debtor_account"`
	ReferenceNumber       string    `json:"reference_number"`
	MerchantCategoryCode  string    `json:"merchant_category_code"`
}

// transactionsResponse keeps the transactions as RawMessage so it can
// preserve the full original JSON in SQLite in addition to the mapped fields.
type transactionsResponse struct {
	Transactions    []json.RawMessage `json:"transactions"`
	ContinuationKey string            `json:"continuation_key"`
}

// TransactionsParams are the query parameters of GET /accounts/{uid}/transactions.
type TransactionsParams struct {
	DateFrom        string // YYYY-MM-DD (inclusive)
	DateTo          string // YYYY-MM-DD (inclusive)
	ContinuationKey string
	Strategy        string
}

// RawTransaction pairs the mapped transaction with its raw JSON.
type RawTransaction struct {
	Tx  Transaction
	Raw json.RawMessage
}

// TransactionsPage is a page of results.
type TransactionsPage struct {
	Transactions    []RawTransaction
	ContinuationKey string
}
