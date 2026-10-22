package a2a

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"myaaw/internal/channel"
	"myaaw/internal/config"

	"github.com/gofiber/fiber/v3"
)

func writeSigningKey(t *testing.T) (string, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "a2a_key.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return path, pub
}

func TestSignVerifyBody(t *testing.T) {
	path, pub := writeSigningKey(t)
	priv, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}

	body := []byte(`{"jsonrpc":"2.0","id":1}`)
	jws, err := SignBody(priv, body)
	if err != nil {
		t.Fatalf("SignBody: %v", err)
	}

	kid, err := VerifyBody(jws, body, map[string]ed25519.PublicKey{KeyID(pub): pub})
	if err != nil {
		t.Fatalf("VerifyBody: %v", err)
	}
	if kid != KeyID(pub) {
		t.Fatalf("kid = %q, want %q", kid, KeyID(pub))
	}

	if _, err := VerifyBody(jws, []byte(`{"jsonrpc":"2.0","id":2}`), map[string]ed25519.PublicKey{KeyID(pub): pub}); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("tampered body err = %v", err)
	}
	if _, err := VerifyBody(jws, body, map[string]ed25519.PublicKey{}); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown key err = %v", err)
	}
	if _, err := VerifyBody("not-a-jws", body, map[string]ed25519.PublicKey{}); !errors.Is(err, ErrMalformedSignature) {
		t.Fatalf("malformed err = %v", err)
	}
}

func TestRPCSignatureRequired(t *testing.T) {
	path, pub := writeSigningKey(t)
	previousPath := config.A2ASigningKeyPath
	previousTrusted := config.A2ATrustedKeys
	previousRequire := config.A2ARequireSignature
	previousToken := config.A2AToken
	previousTokens := config.A2ATokens
	previousCallers := config.A2ACallers
	config.A2ASigningKeyPath = path
	config.A2ATrustedKeys = "peer:" + EncodePublicKey(pub)
	config.A2ARequireSignature = true
	config.A2AToken = ""
	config.A2ATokens = nil
	config.A2ACallers = nil
	t.Cleanup(func() {
		config.A2ASigningKeyPath = previousPath
		config.A2ATrustedKeys = previousTrusted
		config.A2ARequireSignature = previousRequire
		config.A2AToken = previousToken
		config.A2ATokens = previousTokens
		config.A2ACallers = previousCallers
	})

	app := newTestApp(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})
	body := `{"jsonrpc":"2.0","id":1,"method":"tasks/send","params":{"message":{"role":"user","parts":[{"kind":"text","text":"hi"}]}}}`

	if status, _ := doRPC(t, app, "", body); status != fiber.StatusUnauthorized {
		t.Fatalf("unsigned status = %d, want 401", status)
	}

	priv, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	signature, err := SignBody(priv, []byte(body))
	if err != nil {
		t.Fatalf("SignBody: %v", err)
	}

	req := httptest.NewRequest("POST", "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeader, signature)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("signed status = %d, want 200", resp.StatusCode)
	}

	req = httptest.NewRequest("POST", "/a2a", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeader, signature+"x")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("tampered status = %d, want 401", resp.StatusCode)
	}
}
