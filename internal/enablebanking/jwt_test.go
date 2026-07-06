package enablebanking

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func writeTestKey(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	path := filepath.Join(t.TempDir(), "app.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path, key
}

func TestTokenHeadersAndClaims(t *testing.T) {
	path, key := writeTestKey(t)
	priv, err := loadPrivateKey(path)
	if err != nil {
		t.Fatalf("loadPrivateKey: %v", err)
	}
	ts := &tokenSource{appID: "my-app-id", key: priv}

	raw, err := ts.token()
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	parsed, err := jwt.Parse(raw, func(tok *jwt.Token) (any, error) {
		if kid, _ := tok.Header["kid"].(string); kid != "my-app-id" {
			t.Errorf("kid = %q, want my-app-id", kid)
		}
		if tok.Method.Alg() != "RS256" {
			t.Errorf("alg = %q, want RS256", tok.Method.Alg())
		}
		return &key.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("signature verification failed: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("claims of unexpected type")
	}
	if claims["iss"] != "enablebanking.com" {
		t.Errorf("iss = %v", claims["iss"])
	}
	if claims["aud"] != "api.enablebanking.com" {
		t.Errorf("aud = %v", claims["aud"])
	}
	if _, ok := claims["exp"]; !ok {
		t.Errorf("exp claim missing")
	}
}

func TestTokenIsCached(t *testing.T) {
	path, _ := writeTestKey(t)
	priv, _ := loadPrivateKey(path)
	ts := &tokenSource{appID: "x", key: priv}

	a, err := ts.token()
	if err != nil {
		t.Fatal(err)
	}
	b, err := ts.token()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("token not cached: two different values")
	}
}
