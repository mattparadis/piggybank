package enablebanking

import (
	"crypto/rsa"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// tokenSource genera e mette in cache il JWT RS256 usato per autenticarsi verso
// l'API. Il token dura un'ora e viene rigenerato poco prima della scadenza.
type tokenSource struct {
	appID string
	key   *rsa.PrivateKey

	mu     sync.Mutex
	cached string
	exp    time.Time
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}
	return key, nil
}

// token restituisce un JWT valido, rigenerandolo se mancante o quasi scaduto.
func (t *tokenSource) token() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cached != "" && time.Until(t.exp) > time.Minute {
		return t.cached, nil
	}

	now := time.Now()
	exp := now.Add(time.Hour)
	claims := jwt.MapClaims{
		"iss": "enablebanking.com",
		"aud": "api.enablebanking.com",
		"iat": now.Unix(),
		"exp": exp.Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = t.appID

	signed, err := tok.SignedString(t.key)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	t.cached = signed
	t.exp = exp
	return signed, nil
}
