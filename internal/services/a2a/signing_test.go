package a2a

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
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

func mustSigningKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	path, _ := writeSigningKey(t)
	priv, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	return priv
}

func TestKeyUtilities(t *testing.T) {
	path, pub := writeSigningKey(t)
	priv, err := LoadPrivateKey(path)
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}
	if len(priv) == 0 {
		t.Fatal("loaded key is empty")
	}

	encoded := EncodePublicKey(pub)
	decoded, err := DecodePublicKey(encoded)
	if err != nil {
		t.Fatalf("DecodePublicKey: %v", err)
	}
	if !pub.Equal(decoded) {
		t.Fatalf("roundtrip mismatch: %x != %x", pub, decoded)
	}

	if kid := KeyID(pub); kid == "" {
		t.Fatal("KeyID is empty")
	}

	trusted, names := ParseTrustedKeys("peer:" + encoded)
	if len(trusted) != 1 || names[KeyID(pub)] != "peer" {
		t.Fatalf("trusted = %+v, names = %+v", trusted, names)
	}

	if _, err := DecodePublicKey("not-base64!!"); err == nil {
		t.Fatal("invalid key should fail to decode")
	}
}

func TestLoadPrivateKeyErrors(t *testing.T) {
	if _, err := LoadPrivateKey(filepath.Join(t.TempDir(), "missing.pem")); err == nil {
		t.Fatal("missing key file should fail")
	}
	path := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(path, []byte("not pem"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadPrivateKey(path); err == nil {
		t.Fatal("non-PEM file should fail")
	}
}
