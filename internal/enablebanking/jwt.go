package enablebanking

import (
	"crypto/rsa"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// tokenSource generates and caches the RS256 JWT used to authenticate against
// the API. The token lasts one hour and is regenerated shortly before expiry.
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

// token returns a valid JWT, regenerating it if missing or nearly expired.
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
