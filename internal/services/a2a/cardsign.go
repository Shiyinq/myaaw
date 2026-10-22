package a2a

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
)

var (
	ErrUnsignedCard  = errors.New("agent card is not signed")
	ErrCardSignature = errors.New("invalid agent card signature")
)

// SignCard returns a copy of the card with a compact JWS over its RFC 8785
// (JCS) canonical form, per spec §8.4. The signatures field is excluded from
// the signed payload.
func SignCard(card AgentCard, key ed25519.PrivateKey) (AgentCard, error) {
	if len(key) == 0 {
		return AgentCard{}, errors.New("signing key is required")
	}

	payload, err := canonicalCard(card)
	if err != nil {
		return AgentCard{}, err
	}

	header, err := json.Marshal(map[string]string{
		"alg": "EdDSA",
		"typ": "JOSE",
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
// key ID of the first signature that verifies. The protected JWS header must
// carry alg "EdDSA" and a kid that resolves to a trusted key.
func VerifyCard(card AgentCard, trusted map[string]ed25519.PublicKey) (string, error) {
	if len(card.Signatures) == 0 {
		return "", ErrUnsignedCard
	}

	payload, err := canonicalCard(card)
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
			Typ string `json:"typ"`
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

// canonicalCard returns the RFC 8785 canonical form of the card without its
// signatures field. Unset optional fields are omitted from the JSON before
// canonicalization, matching protobuf field-presence semantics per §8.4.1.
func canonicalCard(card AgentCard) ([]byte, error) {
	unsigned := card
	unsigned.Signatures = nil
	raw, err := json.Marshal(unsigned)
	if err != nil {
		return nil, err
	}
	generic, err := jcsFromJSON(raw)
	if err != nil {
		return nil, err
	}
	payload, err := jcsCanonicalize(generic)
	if err != nil {
		return nil, fmt.Errorf("cannot canonicalize agent card: %w", err)
	}
	return payload, nil
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
