package chatgpt

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// RFC 6750 b64token: reject control characters and whitespace before a caller
// can accidentally use an unsafe value in an Authorization header.
var bearerToken = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

type tokenResponse struct {
	AccessToken       string          `json:"access_token"`
	RefreshToken      string          `json:"refresh_token"`
	IDToken           string          `json:"id_token"`
	TokenType         string          `json:"token_type"`
	ExpiresIn         json.Number     `json:"expires_in"`
	Scope             *string         `json:"scope"`
	EarliestRefreshAt json.RawMessage `json:"earliest_refresh_at"`
}

type endpointError struct {
	status int
	code   string // Only allowlisted protocol codes, never server descriptions.
}

func (e *endpointError) Error() string {
	if e.code != "" {
		return fmt.Sprintf("ChatGPT token endpoint rejected request (HTTP %d, %s)", e.status, e.code)
	}
	return fmt.Sprintf("ChatGPT token endpoint rejected request (HTTP %d)", e.status)
}

func terminalRefreshCode(code string) bool {
	switch code {
	case "invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused":
		return true
	}
	return false
}

func (m *Manager) requestTokens(ctx context.Context, form url.Values) (tokenResponse, time.Time, error) {
	var tokens tokenResponse
	form.Set("resource", resource)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.opts.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokens, time.Time{}, errors.New("cannot create ChatGPT token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	started := time.Now()
	resp, err := m.opts.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return tokens, started, ctx.Err()
		}
		return tokens, started, errors.New("ChatGPT token request failed")
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if resp.StatusCode != http.StatusOK {
		e := &endpointError{status: resp.StatusCode}
		if readErr == nil && len(data) <= 1<<20 {
			var body struct {
				Error json.RawMessage `json:"error"`
			}
			if json.Unmarshal(data, &body) == nil {
				var code string
				if json.Unmarshal(body.Error, &code) != nil {
					var nested struct {
						Code string `json:"code"`
					}
					if json.Unmarshal(body.Error, &nested) == nil {
						code = nested.Code
					}
				}
				if terminalRefreshCode(code) || code == "invalid_client" {
					e.code = code
				}
			}
		}
		return tokens, started, e
	}
	if readErr != nil || len(data) > 1<<20 || json.Unmarshal(data, &tokens) != nil {
		return tokens, started, errors.New("invalid ChatGPT token response")
	}
	return tokens, started, nil
}

func parseEarliest(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, nil
	}
	text := string(raw)
	if len(text) > 0 && text[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return time.Time{}, errors.New("invalid ChatGPT earliest refresh time")
		}
		if instant, err := time.Parse(time.RFC3339, text); err == nil && instant.Unix() >= 0 {
			return instant, nil
		}
	}
	seconds, err := strconv.ParseInt(text, 10, 64)
	if err != nil || seconds < 0 || seconds > 253402300799 {
		return time.Time{}, errors.New("invalid ChatGPT earliest refresh time")
	}
	return time.Unix(seconds, 0), nil
}

func applyTokens(r record, t tokenResponse, started time.Time, initial bool) (record, error) {
	seconds, err := t.ExpiresIn.Int64()
	if err != nil || seconds <= 0 || seconds > int64((365*24*time.Hour)/time.Second) || !bearerToken.MatchString(t.AccessToken) || strings.TrimSpace(t.RefreshToken) == "" || !strings.EqualFold(t.TokenType, "Bearer") {
		return record{}, errors.New("incomplete ChatGPT token response")
	}
	if initial && (t.IDToken == "" || t.Scope == nil) {
		return record{}, errors.New("ChatGPT sign-in response lacks identity or granted scopes")
	}
	if t.Scope != nil {
		r.Scopes = strings.Fields(*t.Scope)
	}

	earliest, err := parseEarliest(t.EarliestRefreshAt)
	if err != nil {
		return record{}, err
	}
	r.AccessToken, r.RefreshToken = t.AccessToken, t.RefreshToken
	if t.IDToken != "" {
		r.IDToken = t.IDToken
	}
	r.ExpiresAt = started.Add(time.Duration(seconds) * time.Second)
	r.EarliestRefreshAt = earliest
	if !time.Now().Before(r.ExpiresAt) {
		return record{}, errors.New("ChatGPT token response is already expired")
	}
	return r, nil
}

type identity struct {
	Subject   string  `json:"sub"`
	Email     string  `json:"email"`
	Nonce     string  `json:"nonce"`
	AZP       string  `json:"azp"`
	SID       *string `json:"sid"`
	NotBefore int64   `json:"nbf"`
}

func (m *Manager) verifyIdentity(ctx context.Context, raw, accessToken, clientID, nonce string, initial bool) (identity, error) {
	var claims identity
	verifier := oidc.NewVerifier(m.opts.issuer, m.keys, &oidc.Config{
		ClientID: clientID, SupportedSigningAlgs: []string{oidc.RS256, oidc.ES256},
	})
	id, err := verifier.Verify(ctx, raw)
	if err != nil {
		return claims, errors.New("ChatGPT ID token validation failed")
	}
	if id.Claims(&claims) != nil || strings.TrimSpace(claims.Subject) == "" || id.IssuedAt.IsZero() || id.Expiry.IsZero() || id.IssuedAt.After(time.Now().Add(5*time.Second)) || !id.IssuedAt.Before(id.Expiry) || claims.NotBefore > time.Now().Add(5*time.Second).Unix() {
		return identity{}, errors.New("ChatGPT ID token has invalid required claims")
	}
	if (initial || claims.Nonce != "") && subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
		return identity{}, errors.New("ChatGPT ID token nonce mismatch")
	}
	if (len(id.Audience) > 1 || claims.AZP != "") && claims.AZP != clientID {
		return identity{}, errors.New("ChatGPT ID token authorized-party mismatch")
	}
	if claims.SID != nil && strings.TrimSpace(*claims.SID) == "" {
		return identity{}, errors.New("ChatGPT ID token has invalid session identifier")
	}
	if id.AccessTokenHash != "" && id.VerifyAccessToken(accessToken) != nil {
		return identity{}, errors.New("ChatGPT ID token access-token binding mismatch")
	}
	return claims, nil
}

func (m *Manager) refresh(ctx context.Context, old record) (record, error) {
	tokens, started, err := m.requestTokens(ctx, url.Values{
		"grant_type": {"refresh_token"}, "client_id": {old.ClientID}, "refresh_token": {old.RefreshToken},
	})
	if err != nil {
		var endpoint *endpointError
		if errors.As(err, &endpoint) && (endpoint.status == http.StatusBadRequest || endpoint.status == http.StatusUnauthorized || endpoint.status == http.StatusForbidden) && terminalRefreshCode(endpoint.code) {
			return record{}, ErrReauthorizationRequired
		}
		return record{}, err
	}
	updated, err := applyTokens(old, tokens, started, false)
	if err != nil {
		return record{}, err
	}
	if tokens.IDToken != "" {
		// Once a rotation response arrives, caller cancellation must not discard it
		// while validating its identity. AccessToken persists before returning ctx.Err.
		validationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		claims, err := m.verifyIdentity(validationCtx, tokens.IDToken, tokens.AccessToken, old.ClientID, old.Nonce, false)
		if err != nil {
			return record{}, err
		}
		if claims.Subject != old.Subject {
			return record{}, errors.New("ChatGPT refresh identity changed")
		}
		updated.Email = claims.Email
	}
	return updated, nil
}
