// Package enablebanking is a client for the Enable Banking API (JWT RS256
// authentication, consent initiation, sessions, accounts and transactions).
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

// ErrSessionExpired indicates that the session/consent is no longer valid (HTTP
// 401/403): the `auth` command must be re-run.
var ErrSessionExpired = errors.New("enablebanking: session expired or unauthorized")

// Client talks to the Enable Banking API.
type Client struct {
	baseURL string
	http    *http.Client
	ts      *tokenSource
}

// New builds the client by loading the RSA private key used for JWT signing.
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

// do performs an authenticated request and decodes the JSON response into out.
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

// ListASPSPs lists the banks available for a country (ISO 3166, e.g. "IT").
func (c *Client) ListASPSPs(ctx context.Context, country string) ([]ASPSP, error) {
	q := url.Values{}
	q.Set("country", country)
	var out aspspsResponse
	if err := c.do(ctx, http.MethodGet, "/aspsps", q, nil, &out); err != nil {
		return nil, err
	}
	return out.ASPSPs, nil
}

// StartAuth starts the authorization and returns the consent URL.
func (c *Client) StartAuth(ctx context.Context, req AuthRequest) (*AuthResponse, error) {
	var out AuthResponse
	if err := c.do(ctx, http.MethodPost, "/auth", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateSession exchanges the authorization code for a session.
func (c *Client) CreateSession(ctx context.Context, code string) (*SessionResponse, error) {
	var out SessionResponse
	body := map[string]string{"code": code}
	if err := c.do(ctx, http.MethodPost, "/sessions", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSession retrieves the data of an existing session (useful to validate the
// expiry on the server side).
func (c *Client) GetSession(ctx context.Context, sessionID string) (*SessionResponse, error) {
	var out SessionResponse
	if err := c.do(ctx, http.MethodGet, "/sessions/"+url.PathEscape(sessionID), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetBalances retrieves the balances of an account.
func (c *Client) GetBalances(ctx context.Context, accountUID string) ([]Balance, error) {
	var out balancesResponse
	path := "/accounts/" + url.PathEscape(accountUID) + "/balances"
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Balances, nil
}

// GetTransactionsPage retrieves a page of transactions for an account.
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
