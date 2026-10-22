package a2a

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"myaaw/internal/config"
)

// SignatureHeader carries the compact JWS over the raw request body.
const SignatureHeader = "X-A2A-Signature"

var (
	ErrMalformedSignature = errors.New("malformed signature")
	ErrUnknownKey         = errors.New("unknown signing key")
	ErrInvalidSignature   = errors.New("invalid signature")
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

// SignBody returns a compact JWS (EdDSA) over the raw request body.
func SignBody(key ed25519.PrivateKey, body []byte) (string, error) {
	pub := key.Public().(ed25519.PublicKey)
	header, err := json.Marshal(map[string]string{"alg": "EdDSA", "kid": KeyID(pub)})
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(body)
	signature := ed25519.Sign(key, []byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// VerifyBody validates a compact JWS over body and returns its key ID.
func VerifyBody(jws string, body []byte, trusted map[string]ed25519.PublicKey) (string, error) {
	parts := strings.Split(strings.TrimSpace(jws), ".")
	if len(parts) != 3 {
		return "", ErrMalformedSignature
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrMalformedSignature
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Alg != "EdDSA" || header.Kid == "" {
		return "", ErrMalformedSignature
	}
	pub, ok := trusted[header.Kid]
	if !ok {
		return "", ErrUnknownKey
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", ErrMalformedSignature
	}
	signingInput := parts[0] + "." + parts[1]
	if !ed25519.Verify(pub, []byte(signingInput), signature) {
		return "", ErrInvalidSignature
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !bytes.Equal(payload, body) {
		return "", ErrInvalidSignature
	}
	return header.Kid, nil
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

func (s *Service) loadSigning() {
	s.trustedKeys, s.trustedNames = ParseTrustedKeys(config.A2ATrustedKeys)
	s.requireSignature = config.A2ARequireSignature

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

// VerifySignature validates an incoming request signature and returns the
// trusted caller name when the key is known.
func (s *Service) VerifySignature(jws string, body []byte) (string, error) {
	kid, err := VerifyBody(jws, body, s.trustedKeys)
	if err != nil {
		return "", err
	}
	return s.trustedNames[kid], nil
}
