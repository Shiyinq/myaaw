package chatgpt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

func TestRS256IDTokensAndAlgorithmAllowlist(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: &key.PublicKey, KeyID: "rsa-key", Algorithm: "RS256", Use: "sig",
		}}})
	}))
	defer issuer.Close()
	opts := defaultOptions()
	opts.issuer = issuer.URL
	opts.jwksURL = issuer.URL + "/jwks"
	m, err := newManager(filepath.Join(t.TempDir(), "credentials"), opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, algorithm := range []jose.SignatureAlgorithm{jose.RS256, jose.RS512, jose.HS256} {
		t.Run(string(algorithm), func(t *testing.T) {
			var signingKey any = key
			if algorithm == jose.HS256 {
				signingKey = []byte("untrusted-secret-with-32-characters")
			}
			signer, err := jose.NewSigner(jose.SigningKey{Algorithm: algorithm, Key: signingKey}, (&jose.SignerOptions{}).WithHeader("kid", "rsa-key"))
			if err != nil {
				t.Fatal(err)
			}
			claims, _ := json.Marshal(map[string]any{
				"iss": issuer.URL, "aud": "oaiapp_client", "sub": "subject", "nonce": "nonce",
				"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
			})
			signed, err := signer.Sign(claims)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := signed.CompactSerialize()
			if err != nil {
				t.Fatal(err)
			}
			identity, err := m.verifyIdentity(context.Background(), raw, "access-secret", "oaiapp_client", "nonce", true)
			if algorithm == jose.RS256 {
				if err != nil || identity.Subject != "subject" {
					t.Fatal("official RS256 signature rejected", err)
				}
			} else if err == nil {
				t.Fatal("unapproved signing algorithm accepted")
			}
		})
	}
}

func TestJWKSFailuresAreSanitizedAndRedirectsRejected(t *testing.T) {
	var foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignCalls.Add(1)
	}))
	defer foreign.Close()
	for _, mode := range []string{"redirect", "malformed", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			p := newTestIssuer(t)
			keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "redirect":
					http.Redirect(w, r, foreign.URL, http.StatusFound)
				case "malformed":
					_, _ = w.Write([]byte("malformed-secret"))
				case "unavailable":
					http.Error(w, "server-secret", http.StatusServiceUnavailable)
				}
			}))
			defer keys.Close()
			o := p.options()
			o.jwksURL = keys.URL + "/jwks"
			m, err := newManager(filepath.Join(t.TempDir(), "credentials"), o)
			if err != nil {
				t.Fatal(err)
			}
			raw := p.sign(p.claims("oaiapp_client", "nonce", "subject"), nil)
			if _, err := m.verifyIdentity(context.Background(), raw, "access-secret", "oaiapp_client", "nonce", true); err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), raw) {
				t.Fatal("JWKS failure was accepted or leaked diagnostic contents", err)
			}
			if foreignCalls.Load() != 0 {
				t.Fatal("JWKS redirect followed to another endpoint")
			}
		})
	}
}

func TestValidMultiAudienceAndAccessTokenBinding(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	claims := p.claims("oaiapp_client", "nonce", "subject")
	claims["aud"] = []string{"oaiapp_client", "other-resource"}
	claims["azp"] = "oaiapp_client"
	claims["nbf"] = time.Now().Add(-time.Second).Unix()
	hash := sha256.Sum256([]byte("access-secret"))
	claims["at_hash"] = base64.RawURLEncoding.EncodeToString(hash[:len(hash)/2])
	raw := p.sign(claims, nil)
	if _, err := m.verifyIdentity(context.Background(), raw, "access-secret", "oaiapp_client", "nonce", true); err != nil {
		t.Fatal("valid authorized-party or access-token binding rejected", err)
	}
	if _, err := m.verifyIdentity(context.Background(), raw, "foreign-access", "oaiapp_client", "nonce", true); err == nil {
		t.Fatal("mismatched access-token binding accepted")
	}
}
