package a2a

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
)

var (
	ErrUnsignedCard  = errors.New("agent card is not signed")
	ErrCardSignature = errors.New("invalid agent card signature")
)

// SignCard returns a copy of the card with a compact JWS over its canonical
// JSON form. Signatures are excluded from the signed payload.
//
// The canonical form sorts object keys and removes insignificant whitespace,
// which matches JCS for the data types used by the Agent Card (strings,
// booleans, arrays, and objects).
func SignCard(card AgentCard, key ed25519.PrivateKey) (AgentCard, error) {
	if len(key) == 0 {
		return AgentCard{}, errors.New("signing key is required")
	}

	unsigned := card
	unsigned.Signatures = nil
	payload, err := canonicalCard(unsigned)
	if err != nil {
		return AgentCard{}, err
	}

	header, err := json.Marshal(map[string]string{
		"alg": "EdDSA",
		"kid": KeyID(key.Public().(ed25519.PublicKey)),
	})
	if err != nil {
		return AgentCard{}, err
	}
	protected := base64.RawURLEncoding.EncodeToString(header)
	signingInput := protected + "." + base64.RawURLEncoding.EncodeToString(payload)
	signature := ed25519.Sign(key, []byte(signingInput))

	card.Signatures = []CardSignature{{
		Protected: protected,
		Signature: base64.RawURLEncoding.EncodeToString(signature),
	}}
	return card, nil
}

// VerifyCard checks the card signatures against trusted keys and returns the
// key ID of the first signature that verifies.
func VerifyCard(card AgentCard, trusted map[string]ed25519.PublicKey) (string, error) {
	if len(card.Signatures) == 0 {
		return "", ErrUnsignedCard
	}

	unsigned := card
	unsigned.Signatures = nil
	payload, err := canonicalCard(unsigned)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)

	for _, signature := range card.Signatures {
		headerBytes, err := base64.RawURLEncoding.DecodeString(signature.Protected)
		if err != nil {
			continue
		}
		var header struct {
			Alg string `json:"alg"`
			Kid string `json:"kid"`
		}
		if err := json.Unmarshal(headerBytes, &header); err != nil || header.Alg != "EdDSA" || header.Kid == "" {
			continue
		}
		key, ok := trusted[header.Kid]
		if !ok {
			continue
		}
		sig, err := base64.RawURLEncoding.DecodeString(signature.Signature)
		if err != nil {
			continue
		}
		signingInput := signature.Protected + "." + encodedPayload
		if ed25519.Verify(key, []byte(signingInput), sig) {
			return header.Kid, nil
		}
	}
	return "", ErrCardSignature
}

// ParseTrustedCardKeys is a convenience wrapper over ParseTrustedKeys.
func ParseTrustedCardKeys(spec string) map[string]ed25519.PublicKey {
	keys, _ := ParseTrustedKeys(spec)
	return keys
}

func canonicalCard(card AgentCard) ([]byte, error) {
	raw, err := json.Marshal(card)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(generic); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func (s *Service) signCard(card AgentCard) AgentCard {
	if len(s.signingKey) == 0 {
		return card
	}
	signed, err := SignCard(card, s.signingKey)
	if err != nil {
		log.Printf("a2a: cannot sign agent card: %v", err)
		return card
	}
	return signed
}
