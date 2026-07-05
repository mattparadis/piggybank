// Package auth implementa il comando `auth`: avvia il consenso Enable Banking,
// espone un callback HTTPS (certificati Tailscale), scambia il code con una
// sessione e salva session.json.
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

// Run esegue l'intero flusso di autorizzazione.
func Run(ctx context.Context, cfg *config.Config) error {
	eb := cfg.EnableBanking
	if eb.ASPSPName == "" {
		return fmt.Errorf("enablebanking.aspsp_name mancante: elenca le banche con `aspsps` e imposta il nome esatto")
	}
	as := cfg.AuthServer
	if as.RedirectURL == "" || as.TLSCertPath == "" || as.TLSKeyPath == "" {
		return fmt.Errorf("auth_server.redirect_url, tls_cert_path e tls_key_path sono richiesti")
	}

	client, err := enablebanking.New(eb.BaseURL, eb.ApplicationID, eb.PrivateKeyPath)
	if err != nil {
		return err
	}

	// Percorso del callback ricavato dal redirect_url configurato.
	redirect, err := url.Parse(as.RedirectURL)
	if err != nil {
		return fmt.Errorf("redirect_url non valido: %w", err)
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
		return fmt.Errorf("avvio autorizzazione: %w", err)
	}

	// Attende il callback con il code.
	type result struct {
		code string
		err  error
	}
	resCh := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			http.Error(w, "Autorizzazione negata: "+e, http.StatusBadRequest)
			resCh <- result{err: fmt.Errorf("callback error: %s", e)}
			return
		}
		if got := q.Get("state"); got != state {
			http.Error(w, "state non corrispondente", http.StatusBadRequest)
			resCh <- result{err: errors.New("state non corrispondente (possibile CSRF)")}
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "code mancante", http.StatusBadRequest)
			resCh <- result{err: errors.New("code mancante nel callback")}
			return
		}
		fmt.Fprintln(w, "Autorizzazione completata. Puoi chiudere questa pagina e tornare al terminale.")
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

	fmt.Println("Apri questo link nel browser per autorizzare l'accesso al conto:")
	fmt.Println()
	fmt.Println("   " + authResp.URL)
	fmt.Println()
	fmt.Printf("In ascolto del callback su %s%s ...\n", as.ListenAddr, callbackPath)
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
		return fmt.Errorf("creazione sessione: %w", err)
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
		return fmt.Errorf("salvataggio sessione: %w", err)
	}

	fmt.Printf("Sessione salvata in %s (scade il %s), %d conti autorizzati.\n",
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
