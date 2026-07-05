package enablebanking

import "encoding/json"

// ASPSP è una banca disponibile (GET /aspsps).
type ASPSP struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}

type aspspsResponse struct {
	ASPSPs []ASPSP `json:"aspsps"`
}

// Access definisce fino a quando è valido il consenso.
type Access struct {
	ValidUntil string `json:"valid_until"`
}

// ASPSPRef identifica la banca nelle richieste.
type ASPSPRef struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}

// AuthRequest è il body di POST /auth.
type AuthRequest struct {
	Access      Access   `json:"access"`
	ASPSP       ASPSPRef `json:"aspsp"`
	State       string   `json:"state"`
	RedirectURL string   `json:"redirect_url"`
	PSUType     string   `json:"psu_type"`
}

// AuthResponse è la risposta di POST /auth.
type AuthResponse struct {
	URL             string `json:"url"`
	AuthorizationID string `json:"authorization_id"`
	PSUIDHash       string `json:"psu_id_hash"`
}

// AccountID contiene l'IBAN del conto.
type AccountID struct {
	IBAN string `json:"iban"`
}

// Account è un conto autorizzato restituito da POST /sessions.
type Account struct {
	UID             string    `json:"uid"`
	AccountID       AccountID `json:"account_id"`
	Name            string    `json:"name"`
	Currency        string    `json:"currency"`
	CashAccountType string    `json:"cash_account_type"`
	Product         string    `json:"product"`
}

// SessionResponse è la risposta di POST /sessions e GET /sessions/{id}.
type SessionResponse struct {
	SessionID string    `json:"session_id"`
	Accounts  []Account `json:"accounts"`
	ASPSP     ASPSPRef  `json:"aspsp"`
	PSUType   string    `json:"psu_type"`
	Access    Access    `json:"access"`
}

// Amount è un importo monetario (amount è una stringa nell'API).
type Amount struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

// Party è una controparte (creditor/debtor).
type Party struct {
	Name string `json:"name"`
}

// Transaction è una singola transazione.
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

// transactionsResponse mantiene le transazioni come RawMessage così da poter
// conservare il JSON originale completo in SQLite oltre ai campi mappati.
type transactionsResponse struct {
	Transactions    []json.RawMessage `json:"transactions"`
	ContinuationKey string            `json:"continuation_key"`
}

// TransactionsParams sono i parametri di query di GET /accounts/{uid}/transactions.
type TransactionsParams struct {
	DateFrom        string // YYYY-MM-DD (inclusivo)
	DateTo          string // YYYY-MM-DD (inclusivo)
	ContinuationKey string
	Strategy        string
}

// RawTransaction accoppia la transazione mappata al suo JSON grezzo.
type RawTransaction struct {
	Tx  Transaction
	Raw json.RawMessage
}

// TransactionsPage è una pagina di risultati.
type TransactionsPage struct {
	Transactions    []RawTransaction
	ContinuationKey string
}
