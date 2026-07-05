// Package enablebanking è un client per l'API di Enable Banking (autenticazione
// JWT RS256, avvio consenso, sessioni, conti e transazioni).
package enablebanking

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrSessionExpired indica che la sessione/consenso non è più valida (HTTP
// 401/403): occorre rilanciare il comando `auth`.
var ErrSessionExpired = errors.New("enablebanking: sessione scaduta o non autorizzata")

// Client parla con l'API di Enable Banking.
type Client struct {
	baseURL string
	http    *http.Client
	ts      *tokenSource
}

// New costruisce il client caricando la chiave privata RSA per la firma JWT.
func New(baseURL, appID, privateKeyPath string) (*Client, error) {
	key, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 60 * time.Second},
		ts:      &tokenSource{appID: appID, key: key},
	}, nil
}

// do esegue una richiesta autenticata e decodifica la risposta JSON in out.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return err
	}
	tok, err := c.ts.token()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w (%s %s): %s", ErrSessionExpired, method, path, snippet(data))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return fmt.Errorf("enablebanking %s %s: status %d: %s", method, path, resp.StatusCode, snippet(data))
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

func snippet(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}

// ListASPSPs elenca le banche disponibili per un paese (ISO 3166, es. "IT").
func (c *Client) ListASPSPs(ctx context.Context, country string) ([]ASPSP, error) {
	q := url.Values{}
	q.Set("country", country)
	var out aspspsResponse
	if err := c.do(ctx, http.MethodGet, "/aspsps", q, nil, &out); err != nil {
		return nil, err
	}
	return out.ASPSPs, nil
}

// StartAuth avvia l'autorizzazione e restituisce l'URL di consenso.
func (c *Client) StartAuth(ctx context.Context, req AuthRequest) (*AuthResponse, error) {
	var out AuthResponse
	if err := c.do(ctx, http.MethodPost, "/auth", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateSession scambia il code di autorizzazione con una sessione.
func (c *Client) CreateSession(ctx context.Context, code string) (*SessionResponse, error) {
	var out SessionResponse
	body := map[string]string{"code": code}
	if err := c.do(ctx, http.MethodPost, "/sessions", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSession recupera i dati di una sessione esistente (utile per validare la
// scadenza lato server).
func (c *Client) GetSession(ctx context.Context, sessionID string) (*SessionResponse, error) {
	var out SessionResponse
	if err := c.do(ctx, http.MethodGet, "/sessions/"+url.PathEscape(sessionID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTransactionsPage recupera una pagina di transazioni di un conto.
func (c *Client) GetTransactionsPage(ctx context.Context, accountUID string, p TransactionsParams) (*TransactionsPage, error) {
	q := url.Values{}
	if p.DateFrom != "" {
		q.Set("date_from", p.DateFrom)
	}
	if p.DateTo != "" {
		q.Set("date_to", p.DateTo)
	}
	if p.ContinuationKey != "" {
		q.Set("continuation_key", p.ContinuationKey)
	}
	if p.Strategy != "" {
		q.Set("strategy", p.Strategy)
	}

	var raw transactionsResponse
	path := "/accounts/" + url.PathEscape(accountUID) + "/transactions"
	if err := c.do(ctx, http.MethodGet, path, q, nil, &raw); err != nil {
		return nil, err
	}

	page := &TransactionsPage{ContinuationKey: raw.ContinuationKey}
	for _, rt := range raw.Transactions {
		var tx Transaction
		if err := json.Unmarshal(rt, &tx); err != nil {
			return nil, fmt.Errorf("decode transaction: %w", err)
		}
		page.Transactions = append(page.Transactions, RawTransaction{Tx: tx, Raw: rt})
	}
	return page, nil
}
