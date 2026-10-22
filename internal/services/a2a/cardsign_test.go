package a2a

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"myaaw/internal/config"
)

func TestSignAndVerifyCard(t *testing.T) {
	path, pub := writeSigningKey(t)
	priv, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}

	card := BuildCard("alice", "http://localhost:8081", nil, true)

	signed, err := SignCard(card, priv)
	if err != nil {
		t.Fatalf("SignCard: %v", err)
	}
	if len(signed.Signatures) != 1 {
		t.Fatalf("signatures = %+v", signed.Signatures)
	}

	protected, err := base64.RawURLEncoding.DecodeString(signed.Signatures[0].Protected)
	if err != nil {
		t.Fatalf("decode protected header: %v", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(protected, &header); err != nil {
		t.Fatalf("parse protected header: %v", err)
	}
	if header.Alg != "EdDSA" || header.Typ != "JOSE" || header.Kid != KeyID(pub) {
		t.Fatalf("protected header = %+v", header)
	}

	trusted := map[string]ed25519.PublicKey{KeyID(pub): pub}
	kid, err := VerifyCard(signed, trusted)
	if err != nil {
		t.Fatalf("VerifyCard: %v", err)
	}
	if kid != KeyID(pub) {
		t.Fatalf("kid = %q, want %q", kid, KeyID(pub))
	}

	tampered := signed
	tampered.Name = "mallory"
	if _, err := VerifyCard(tampered, trusted); !errors.Is(err, ErrCardSignature) {
		t.Fatalf("tampered err = %v", err)
	}

	if _, err := VerifyCard(card, trusted); !errors.Is(err, ErrUnsignedCard) {
		t.Fatalf("unsigned err = %v", err)
	}
}

func TestCardEndpointSignsCard(t *testing.T) {
	path, pub := writeSigningKey(t)
	previousPath := config.A2ASigningKeyPath
	previousTrusted := config.A2ATrustedKeys
	config.A2ASigningKeyPath = path
	config.A2ATrustedKeys = "peer:" + EncodePublicKey(pub)
	t.Cleanup(func() {
		config.A2ASigningKeyPath = previousPath
		config.A2ATrustedKeys = previousTrusted
	})

	app := newTestApp(t, &fakeRunner{})
	req := httptest.NewRequest("GET", "/.well-known/agent-card.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if len(card.Signatures) != 1 {
		t.Fatalf("signatures = %+v", card.Signatures)
	}

	kid, err := VerifyCard(card, ParseTrustedCardKeys(config.A2ATrustedKeys))
	if err != nil {
		t.Fatalf("VerifyCard: %v", err)
	}
	if kid != KeyID(pub) {
		t.Fatalf("kid = %q, want %q", kid, KeyID(pub))
	}
}

func TestCardEndpointUnsignedWithoutKey(t *testing.T) {
	previousPath := config.A2ASigningKeyPath
	config.A2ASigningKeyPath = ""
	t.Cleanup(func() { config.A2ASigningKeyPath = previousPath })

	app := newTestApp(t, &fakeRunner{})
	req := httptest.NewRequest("GET", "/.well-known/agent-card.json", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()

	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if len(card.Signatures) != 0 {
		t.Fatalf("signatures = %+v", card.Signatures)
	}
}
