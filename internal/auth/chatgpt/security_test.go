package chatgpt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallbackReplayIsOneShot(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	a := begin(t, m, "")
	entered, release := make(chan struct{}), make(chan struct{})
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		tokenReply(w, p.sign(p.claims("oaiapp_new", a.nonce, "subject"), nil), "access-secret", "refresh-secret", nil)
	})
	result := make(chan int, 1)
	q := url.Values{"state": {a.state}, "code": {"code"}, "client_id": {"oaiapp_new"}}
	go func() { result <- sendCallback(t, a, q) }()
	<-entered
	status := sendCallback(t, a, q)
	close(release)
	if status != 400 || <-result != 200 {
		t.Fatal("replayed callback accepted or original callback rejected")
	}
	if _, err := waitFor(t, a); err != nil || p.calls.Load() != 1 {
		t.Fatal("callback was not redeemed exactly once", err)
	}
	// Exercise replay after completion even if listener shutdown already ran.
	req := httptest.NewRequest(http.MethodGet, a.redirect+"?"+q.Encode(), nil)
	w := httptest.NewRecorder()
	a.callback(w, req)
	if w.Code != 400 || p.calls.Load() != 1 {
		t.Fatal("completed callback replayed")
	}
}

func TestCancelInFlightDoesNotSave(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	old := seed(t, m)
	a := begin(t, m, old.ID)
	entered, release := make(chan struct{}), make(chan struct{})
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		tokenReply(w, p.sign(p.claims(old.ClientID, a.nonce, old.Subject), nil), "new-access-secret", "new-refresh-secret", nil)
	})
	finished := make(chan int, 1)
	go func() {
		finished <- sendCallback(t, a, url.Values{"state": {a.state}, "code": {"code"}})
	}()
	<-entered
	a.Cancel()
	close(release)
	if _, err := waitFor(t, a); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel did not unblock waiter", err)
	}
	if <-finished != 400 {
		t.Fatal("cancelled callback reported success")
	}
	got, err := m.Get(old.ID)
	if err != nil || got.RefreshToken != old.RefreshToken {
		t.Fatal("cancelled attempt overwrote credentials", err)
	}
}

func TestCallbackRejectsWrongMethodAndHostWithoutConsumingAttempt(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	a := begin(t, m, "")
	for _, test := range []struct{ method, target string }{
		{http.MethodPost, a.redirect},
		{http.MethodGet, "http://localhost/auth/callback"},
		{http.MethodGet, a.redirect + "/wrong"},
	} {
		w := httptest.NewRecorder()
		a.callback(w, httptest.NewRequest(test.method, test.target, nil))
		if w.Code != 400 || a.Status().Status != "pending" {
			t.Fatal("unrelated request consumed attempt")
		}
	}
}

func TestJWKSCacheAndKeyRotation(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	for i, kid := range []string{"signing-key", "signing-key", "rotated-key"} {
		p.mu.Lock()
		p.kid = kid
		if i == 2 {
			p.key, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		}
		p.mu.Unlock()
		raw := p.sign(p.claims("oaiapp_client", "nonce", "subject"), nil)
		if _, err := m.verifyIdentity(context.Background(), raw, "access", "oaiapp_client", "nonce", true); err != nil {
			t.Fatal("key verification failed", err)
		}
		want := int32(1)
		if i == 2 {
			want = 2
		}
		if p.jwks.Load() != want {
			t.Fatalf("JWKS cache/rotation: got %d fetches, want %d", p.jwks.Load(), want)
		}
	}
}

func TestSameEmailRegistrationsStaySeparate(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	var ids []string
	for _, client := range []string{"oaiapp_one", "oaiapp_two"} {
		a := begin(t, m, "")
		p.setHandler(func(w http.ResponseWriter, r *http.Request) {
			tokenReply(w, p.sign(p.claims(client, a.nonce, "same-subject"), nil), "access-"+client, "refresh-"+client, nil)
		})
		sendCallback(t, a, url.Values{"state": {a.state}, "code": {"code"}, "client_id": {client}})
		credential, err := waitFor(t, a)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, credential.ID)
	}
	if ids[0] == ids[1] {
		t.Fatal("registrations deduplicated by email or subject")
	}
	for i, id := range ids {
		got, err := m.Get(id)
		if err != nil || got.Email != "user@example.com" || got.RefreshToken != []string{"refresh-oaiapp_one", "refresh-oaiapp_two"}[i] {
			t.Fatal("registration mixed with another account", err)
		}
	}
}

func TestRefreshIdentityValidationAndScopeRetention(t *testing.T) {
	for _, mode := range []string{"omitted identity and scope", "new valid identity", "bad subject", "bad audience", "bad nonce", "reduced grant", "missing rotation"} {
		t.Run(mode, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			old := seed(t, m)
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				body := map[string]any{
					"access_token": "new-access-secret", "refresh_token": "new-refresh-secret", "token_type": "Bearer", "expires_in": 3600,
				}
				if mode != "omitted identity and scope" {
					claims := p.claims(old.ClientID, old.Nonce, old.Subject)
					delete(claims, "nonce") // Refresh may omit nonce, unlike code exchange.
					switch mode {
					case "bad subject":
						claims["sub"] = "foreign-subject"
					case "bad audience":
						claims["aud"] = "oaiapp_foreign"
					case "bad nonce":
						claims["nonce"] = "foreign-nonce"
					case "reduced grant":
						body["scope"] = "openid email"
					case "missing rotation":
						delete(body, "refresh_token")
					}
					body["id_token"] = p.sign(claims, nil)
				}
				_ = json.NewEncoder(w).Encode(body)
			})
			token, err := m.AccessToken(context.Background(), old.ID)
			wantSuccess := mode == "omitted identity and scope" || mode == "new valid identity"
			if wantSuccess && (err != nil || token != "new-access-secret") {
				t.Fatal("valid refresh rejected", err)
			}
			if !wantSuccess && (err == nil || token != "") {
				t.Fatal("invalid refresh accepted")
			}
			got, err := m.Get(old.ID)
			if err != nil {
				t.Fatal(err)
			}
			if wantSuccess {
				if got.RefreshToken != "new-refresh-secret" || requireScopes(got.Scopes) != nil {
					t.Fatal("rotation or scope retention failed")
				}
			} else if mode == "reduced grant" {
				if got.RefreshToken != "new-refresh-secret" || requireScopes(got.Scopes) == nil {
					t.Fatal("reduced grant lost rotation or kept plan permission")
				}
			} else if got.RefreshToken != old.RefreshToken {
				t.Fatal("invalid refresh replaced saved binding")
			}
		})
	}
}

func TestTemporaryRefreshFailureNeverDestroysCredentials(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	old := seed(t, m)
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"secret"}`)
	})
	if _, err := m.AccessToken(context.Background(), old.ID); err == nil {
		t.Fatal("temporary error accepted")
	}
	got, err := m.Get(old.ID)
	if err != nil || got.RefreshToken != old.RefreshToken {
		t.Fatal("temporary infrastructure error destroyed credentials", err)
	}
}

func TestTokenRedirectDoesNotForwardCredentials(t *testing.T) {
	var foreignCalls atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignCalls.Add(1)
	}))
	defer foreign.Close()
	p := newTestIssuer(t)
	m := managerFor(t, p)
	old := seed(t, m)
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL, http.StatusTemporaryRedirect)
	})
	if _, err := m.AccessToken(context.Background(), old.ID); err == nil || foreignCalls.Load() != 0 {
		t.Fatal("credential-bearing redirect was followed")
	}
}

func TestIncompleteTokenResponses(t *testing.T) {
	scope := scopeList
	base := tokenResponse{
		AccessToken: "access-secret", RefreshToken: "refresh-secret", IDToken: "id-secret",
		TokenType: "Bearer", ExpiresIn: "3600", Scope: &scope,
	}
	for name, mutate := range map[string]func(*tokenResponse){
		"access":           func(t *tokenResponse) { t.AccessToken = "" },
		"refresh":          func(t *tokenResponse) { t.RefreshToken = "" },
		"header injection": func(t *tokenResponse) { t.AccessToken = "secret\r\nHeader: value" },
		"blank access":     func(t *tokenResponse) { t.AccessToken = " " },
		"blank refresh":    func(t *tokenResponse) { t.RefreshToken = " " },
		"id":               func(t *tokenResponse) { t.IDToken = "" },
		"type":             func(t *tokenResponse) { t.TokenType = "MAC" },
		"scopes":           func(t *tokenResponse) { t.Scope = nil },
		"expiry":           func(t *tokenResponse) { t.ExpiresIn = "-1" },
		"overflow":         func(t *tokenResponse) { t.ExpiresIn = "9223372036854775807" },
		"earliest":         func(t *tokenResponse) { t.EarliestRefreshAt = json.RawMessage(`"invalid-secret"`) },
	} {
		t.Run(name, func(t *testing.T) {
			tokens := base
			mutate(&tokens)
			if _, err := applyTokens(record{}, tokens, time.Now(), true); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("incomplete response accepted or leaked data", err)
			}
		})
	}
}

func TestAtomicReplacementAndPermissionRepair(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	old := seed(t, m)
	if err := os.Chmod(m.accountPath(old.ID), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(old.ID); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(m.accountPath(old.ID))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("file permissions not repaired", err)
	}
	old.RefreshToken = "replacement-refresh-secret"
	if err := atomicJSON(m.accountPath(old.ID), old); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(m.dir, "accounts"))
	if err != nil || len(entries) != 1 || entries[0].Name() != old.ID+".json" {
		t.Fatal("atomic replacement left temporary credential files", err)
	}
	got, err := m.Get(old.ID)
	if err != nil || got.RefreshToken != old.RefreshToken {
		t.Fatal("atomic replacement lost contents", err)
	}
}
