package a2a

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"myaaw/internal/config"
)

// EncodePublicKey renders a raw Ed25519 public key as unpadded base64url.
func EncodePublicKey(pub ed25519.PublicKey) string {
	return base64.RawURLEncoding.EncodeToString(pub)
}

// DecodePublicKey accepts raw Ed25519 keys in base64url or base64.
func DecodePublicKey(value string) (ed25519.PublicKey, error) {
	value = strings.TrimSpace(value)
	for _, decoder := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.StdEncoding} {
		raw, err := decoder.DecodeString(value)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			continue
		}
		return ed25519.PublicKey(raw), nil
	}
	return nil, fmt.Errorf("public key must be %d raw bytes in base64", ed25519.PublicKeySize)
}

// KeyID derives a short stable identifier from a public key.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return base64.RawURLEncoding.EncodeToString(sum[:8])
}

// LoadPrivateKey reads an Ed25519 PKCS#8 PEM file.
func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("signing key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("signing key must be Ed25519 PKCS#8")
	}
	return key, nil
}

// ParseTrustedKeys parses "name:base64key,name2:base64key" and returns maps
// keyed by key ID.
func ParseTrustedKeys(spec string) (map[string]ed25519.PublicKey, map[string]string) {
	keys := make(map[string]ed25519.PublicKey)
	names := make(map[string]string)
	for _, entry := range strings.Split(spec, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 {
			log.Printf("a2a: ignoring malformed trusted key entry")
			continue
		}
		name := strings.TrimSpace(parts[0])
		pub, err := DecodePublicKey(parts[1])
		if err != nil {
			log.Printf("a2a: ignoring trusted key for %q: %v", name, err)
			continue
		}
		kid := KeyID(pub)
		keys[kid] = pub
		names[kid] = name
	}
	return keys, names
}

// ParseTrustedCardKeys is a convenience wrapper over ParseTrustedKeys.
func ParseTrustedCardKeys(spec string) map[string]ed25519.PublicKey {
	keys, _ := ParseTrustedKeys(spec)
	return keys
}

func (s *Service) loadSigning() {
	s.trustedKeys, s.trustedNames = ParseTrustedKeys(config.A2ATrustedKeys)

	path := strings.TrimSpace(config.A2ASigningKeyPath)
	if path == "" {
		return
	}
	key, err := LoadPrivateKey(path)
	if err != nil {
		log.Printf("a2a: cannot load signing key %s: %v", path, err)
		return
	}
	s.signingKey = key
}
