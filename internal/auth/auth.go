// Package auth implements the `auth` command: it starts the Enable Banking
// consent flow, exposes an HTTPS callback (Tailscale certificates), exchanges
// the code for a session and saves session.json.
package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"expense_monitor/internal/config"
	"expense_monitor/internal/enablebanking"
	"expense_monitor/internal/session"
)

// Run performs the entire authorization flow.
func Run(ctx context.Context, cfg *config.Config) error {
	eb := cfg.EnableBanking
	if eb.ASPSPName == "" {
		return fmt.Errorf("enablebanking.aspsp_name is missing: list banks with `aspsps` and set the exact name")
	}
	as := cfg.AuthServer
	if as.RedirectURL == "" || as.TLSCertPath == "" || as.TLSKeyPath == "" {
		return fmt.Errorf("auth_server.redirect_url, tls_cert_path and tls_key_path are required")
	}

	client, err := enablebanking.New(eb.BaseURL, eb.ApplicationID, eb.PrivateKeyPath)
	if err != nil {
		return err
	}

	// Callback path derived from the configured redirect_url.
	redirect, err := url.Parse(as.RedirectURL)
	if err != nil {
		return fmt.Errorf("invalid redirect_url: %w", err)
	}
	callbackPath := redirect.Path
	if callbackPath == "" {
		callbackPath = "/"
	}

	state, err := randomState()
	if err != nil {
		return err
	}

	validUntil := time.Now().Add(time.Duration(eb.ConsentDays) * 24 * time.Hour).UTC().Format(time.RFC3339)
	authResp, err := client.StartAuth(ctx, enablebanking.AuthRequest{
		Access:      enablebanking.Access{ValidUntil: validUntil},
		ASPSP:       enablebanking.ASPSPRef{Name: eb.ASPSPName, Country: eb.Country},
		State:       state,
		RedirectURL: as.RedirectURL,
		PSUType:     eb.PSUType,
	})
	if err != nil {
		return fmt.Errorf("start authorization: %w", err)
	}

	// Wait for the callback carrying the code.
	type result struct {
		code string
		err  error
	}
	resCh := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			http.Error(w, "Authorization denied: "+e, http.StatusBadRequest)
			resCh <- result{err: fmt.Errorf("callback error: %s", e)}
			return
		}
		if got := q.Get("state"); got != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			resCh <- result{err: errors.New("state mismatch (possible CSRF)")}
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			resCh <- result{err: errors.New("missing code in callback")}
			return
		}
		fmt.Fprintln(w, "Authorization complete. You can close this page and return to the terminal.")
		resCh <- result{code: code}
	})

	srv := &http.Server{Addr: as.ListenAddr, Handler: mux}
	srvErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServeTLS(as.TLSCertPath, as.TLSKeyPath)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()

	fmt.Println("Open this link in your browser to authorize account access:")
	fmt.Println()
	fmt.Println("   " + authResp.URL)
	fmt.Println()
	fmt.Printf("Listening for the callback on %s%s ...\n", as.ListenAddr, callbackPath)
	openBrowser(authResp.URL)

	var code string
	select {
	case <-ctx.Done():
		shutdown(srv)
		return ctx.Err()
	case err := <-srvErr:
		return fmt.Errorf("callback server: %w", err)
	case res := <-resCh:
		shutdown(srv)
		if res.err != nil {
			return res.err
		}
		code = res.code
	}

	sessResp, err := client.CreateSession(ctx, code)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	sess := &session.Session{
		SessionID: sessResp.SessionID,
		CreatedAt: time.Now().UTC(),
	}
	if t, err := time.Parse(time.RFC3339, sessResp.Access.ValidUntil); err == nil {
		sess.ValidUntil = t
	}
	for _, a := range sessResp.Accounts {
		sess.Accounts = append(sess.Accounts, session.Account{
			UID:      a.UID,
			IBAN:     a.AccountID.IBAN,
			Name:     a.Name,
			Currency: a.Currency,
		})
	}
	if err := session.Save(cfg.Storage.SessionPath, sess); err != nil {
		return fmt.Errorf("save session: %w", err)
	}

	fmt.Printf("Session saved to %s (expires %s), %d authorized accounts.\n",
		cfg.Storage.SessionPath, sess.ValidUntil.Format(time.RFC3339), len(sess.Accounts))
	return nil
}

func shutdown(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// UUID v4
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func openBrowser(rawURL string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, append(args, rawURL)...).Start()
}
