package chatgpt

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
)

type testIssuer struct {
	server *httptest.Server
	key    *ecdsa.PrivateKey
	mu     sync.Mutex
	handle http.HandlerFunc
	calls  atomic.Int32
	jwks   atomic.Int32
	kid    string
}

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := &testIssuer{key: key, kid: "signing-key"}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/jwks.json":
			p.jwks.Add(1)
			p.mu.Lock()
			key := jose.JSONWebKey{Key: &p.key.PublicKey, KeyID: p.kid, Algorithm: "ES256", Use: "sig"}
			p.mu.Unlock()
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{key}})
		case "/api/accounts/oauth/token":
			p.calls.Add(1)
			if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Error("wrong token request method or encoding")
			}
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("resource") != resource || r.Header.Get("Authorization") != "" || r.Form.Has("client_secret") {
				t.Error("wrong resource or unexpected client authentication")
			}
			p.mu.Lock()
			handler := p.handle
			p.mu.Unlock()
			if handler == nil {
				http.Error(w, "unexpected token request", 500)
				return
			}
			handler(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *testIssuer) setHandler(handler http.HandlerFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handle = handler
}

func (p *testIssuer) options() options {
	o := defaultOptions()
	o.issuer = p.server.URL
	o.authorizeURL = p.server.URL + "/api/accounts/authorize"
	o.tokenURL = p.server.URL + "/api/accounts/oauth/token"
	o.jwksURL = p.server.URL + "/.well-known/jwks.json"
	return o
}

func managerFor(t *testing.T, p *testIssuer) *Manager {
	t.Helper()
	m, err := newManager(filepath.Join(t.TempDir(), "credentials"), p.options())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func begin(t *testing.T, m *Manager, id string) *Attempt {
	t.Helper()
	a, err := m.Start(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Cancel)
	return a
}

func authParams(t *testing.T, a *Attempt) url.Values {
	t.Helper()
	u, err := url.Parse(a.AuthorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func sendCallback(t *testing.T, a *Attempt, values url.Values) int {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(a.redirect + "?" + values.Encode())
	if err != nil {
		t.Error(err)
		return 0
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
		return 0
	}
	if strings.Contains(string(body), "secret") || resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Error("unsafe callback response")
	}
	return resp.StatusCode
}

func waitFor(t *testing.T, a *Attempt) (Credential, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.Wait(ctx)
}

func (p *testIssuer) claims(clientID, nonce, subject string) map[string]any {
	return map[string]any{
		"iss": p.server.URL, "aud": []string{clientID}, "nonce": nonce,
		"sub": subject, "email": "user@example.com", "iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
}

func (p *testIssuer) sign(claims map[string]any, key *ecdsa.PrivateKey) string {
	p.mu.Lock()
	if key == nil {
		key = p.key
	}
	kid := p.kid
	p.mu.Unlock()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", kid))
	if err != nil {
		panic(err)
	}
	data, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}
	signed, err := signer.Sign(data)
	if err != nil {
		panic(err)
	}
	raw, err := signed.CompactSerialize()
	if err != nil {
		panic(err)
	}
	return raw
}

func tokenReply(w http.ResponseWriter, idToken, access, refresh string, extras map[string]any) {
	body := map[string]any{
		"access_token": access, "refresh_token": refresh, "token_type": "Bearer",
		"expires_in": 3600, "scope": scopeList,
	}
	if idToken != "" {
		body["id_token"] = idToken
	}
	for key, value := range extras {
		body[key] = value
	}
	_ = json.NewEncoder(w).Encode(body)
}

func seed(t *testing.T, m *Manager) record {
	t.Helper()
	r := record{Credential: Credential{
		ID: uuid.NewString(), ClientID: "oaiapp_saved", Subject: "saved-subject", Email: "user@example.com",
		AccessToken: "old-access-secret", RefreshToken: "old-refresh-secret", IDToken: "old-id-secret",
		ExpiresAt: time.Now().Add(-time.Minute), Scopes: strings.Fields(scopeList),
	}, Nonce: "original-nonce"}
	if err := atomicJSON(m.accountPath(r.ID), r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestContractAndDefaultHostRoot(t *testing.T) {
	if ProviderType != "openai_chatgpt" || Label != "ChatGPT Subscription" {
		t.Fatal("public provider constants changed")
	}
	o := defaultOptions()
	if o.authorizeURL != issuerURL+"/api/accounts/authorize" || o.tokenURL != issuerURL+"/api/accounts/oauth/token" || o.jwksURL != issuerURL+"/.well-known/jwks.json" || o.issuer != "https://auth.openai.com" {
		t.Fatal("incorrect official endpoints")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("MYAAW_AGENT", "some-other-agent")
	m, err := DefaultManager()
	if err != nil {
		t.Fatal(err)
	}
	if m.dir != filepath.Join(home, ".myaaw", "credentials", "chatgpt") {
		t.Fatalf("not host-global: %s", m.dir)
	}
	again, err := DefaultManager()
	if err != nil || again.hostID != m.hostID {
		t.Fatal("host ID not stable", err)
	}
	if _, err := NewManager(""); err == nil {
		t.Fatal("empty directory accepted")
	}
}

func TestRegistrationReauthorizationAndPermissions(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	a := begin(t, m, "")
	q := authParams(t, a)
	if q.Get("client_id") != "dynamic_agent_client" || q.Get("agent_name_hint") != "Myaaw" || q.Get("ext_agent_host_id") != m.hostID || q.Get("scope") != scopeList || q.Get("resource") != resource || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" {
		t.Fatal("incorrect initial authorization parameters")
	}
	redirect, _ := url.Parse(q.Get("redirect_uri"))
	if redirect.Scheme != "http" || redirect.Hostname() != "127.0.0.1" || redirect.Port() == "" || redirect.Path != "/auth/callback" {
		t.Fatal("incorrect loopback redirect")
	}
	if a.Status().Status != "pending" {
		t.Fatal("attempt not pending")
	}
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		challenge := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("client_id") != "oaiapp_first" || r.Form.Get("code") != "code-secret" || r.Form.Get("redirect_uri") != q.Get("redirect_uri") || base64.RawURLEncoding.EncodeToString(challenge[:]) != q.Get("code_challenge") {
			t.Error("incorrect PKCE code exchange")
		}
		tokenReply(w, p.sign(p.claims("oaiapp_first", q.Get("nonce"), "subject-1"), nil), "access-secret", "refresh-secret", map[string]any{"earliest_refresh_at": time.Now().Add(30 * time.Minute).Unix()})
	})
	if status := sendCallback(t, a, url.Values{"state": {q.Get("state")}, "code": {"code-secret"}, "client_id": {"oaiapp_first"}}); status != 200 {
		t.Fatal("callback rejected")
	}
	credential, err := waitFor(t, a)
	if err != nil || credential.ClientID != "oaiapp_first" || credential.Subject != "subject-1" || credential.ID == "" || credential.ID == a.ID {
		t.Fatal("incorrect credential result", err)
	}
	if a.Status().Status != "connected" || a.Status().CredentialID != credential.ID || a.Status().Email != credential.Email {
		t.Fatal("incorrect safe status")
	}
	statusJSON, _ := json.Marshal(a.Status())
	for _, secret := range []string{"access-secret", "refresh-secret", credential.IDToken, a.nonce, a.verifier, a.state} {
		if strings.Contains(string(statusJSON), secret) {
			t.Fatal("status leaked secret")
		}
	}
	loaded, err := m.Get(credential.ID)
	if err != nil || loaded.RefreshToken != "refresh-secret" {
		t.Fatal("credential not persisted", err)
	}
	token, err := m.AccessToken(context.Background(), credential.ID)
	if err != nil || token != "access-secret" || p.calls.Load() != 1 {
		t.Fatal("valid access token unexpectedly refreshed", err)
	}
	if runtime.GOOS != "windows" {
		for path, mode := range map[string]os.FileMode{m.dir: 0o700, filepath.Join(m.dir, "accounts"): 0o700, filepath.Join(m.dir, "host.json"): 0o600, filepath.Join(m.dir, ".lock"): 0o600, m.accountPath(credential.ID): 0o600} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != mode {
				t.Errorf("wrong file permissions for %s: %v", path, err)
			}
		}
	}
	other, err := newManager(m.dir, p.options())
	if err != nil || other.hostID != m.hostID {
		t.Fatal("new manager lost host ID", err)
	}
	b := begin(t, other, credential.ID)
	bq := authParams(t, b)
	if bq.Get("client_id") != credential.ClientID || bq.Has("agent_name_hint") || bq.Get("id_token_hint") != credential.IDToken || bq.Get("login_hint") != credential.Email || bq.Get("ext_agent_host_id") != m.hostID {
		t.Fatal("incorrect returning sign-in parameters")
	}
	for _, name := range []string{"state", "nonce", "code_challenge"} {
		if q.Get(name) == bq.Get(name) || bq.Get(name) == "" {
			t.Error("reused or empty", name)
		}
	}
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		tokenReply(w, p.sign(p.claims(credential.ClientID, bq.Get("nonce"), credential.Subject), nil), "new-access-secret", "new-refresh-secret", nil)
	})
	// Reauthorization may omit client_id, but must not change the local ID.
	sendCallback(t, b, url.Values{"state": {bq.Get("state")}, "code": {"new-code"}})
	updated, err := waitFor(t, b)
	if err != nil || updated.ID != credential.ID || updated.RefreshToken != "new-refresh-secret" {
		t.Fatal("reauthorization failed", err)
	}
	if err := m.Delete(credential.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(credential.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted credential remains")
	}
	if _, err := NewManager(m.dir); err != nil {
		t.Fatal("deleting account damaged host", err)
	}
}

func TestRejectCallbackBeforeExchange(t *testing.T) {
	cases := map[string]func(url.Values){
		"bad state":        func(q url.Values) { q.Set("state", "wrong-state-secret") },
		"missing state":    func(q url.Values) { q.Del("state") },
		"duplicate state":  func(q url.Values) { q.Add("state", q.Get("state")) },
		"no issued client": func(q url.Values) { q.Del("client_id") },
		"dynamic client":   func(q url.Values) { q.Set("client_id", "dynamic_agent_client") },
		"foreign client":   func(q url.Values) { q.Set("client_id", "foreign-client") },
		"duplicate client": func(q url.Values) { q.Add("client_id", "oaiapp_other") },
		"missing code":     func(q url.Values) { q.Del("code") },
		"duplicate code":   func(q url.Values) { q.Add("code", "other-secret") },
		"access denied":    func(q url.Values) { q.Set("error", "access_denied"); q.Set("error_description", "secret") },
		"untrusted error":  func(q url.Values) { q.Set("error", "secret") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			a := begin(t, m, "")
			q := url.Values{"state": {a.state}, "code": {"code-secret"}, "client_id": {"oaiapp_new"}}
			mutate(q)
			if sendCallback(t, a, q) != 400 {
				t.Fatal("bad callback accepted")
			}
			if strings.Contains(name, "state") {
				if a.Status().Status != "pending" || p.calls.Load() != 0 {
					t.Fatal("untrusted state consumed attempt")
				}
				a.Cancel()
			}
			if _, err := waitFor(t, a); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("bad callback succeeded or error leaked data")
			}
			if p.calls.Load() != 0 || a.Status().Status != "failed" {
				t.Fatal("exchanged untrusted callback")
			}
		})
	}
}

func TestReturningClientAndSubjectCannotChange(t *testing.T) {
	for _, mode := range []string{"client", "subject", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			old := seed(t, m)
			a := begin(t, m, old.ID)
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				sub := old.Subject
				if mode == "subject" {
					sub = "different-subject"
				}
				tokenReply(w, p.sign(p.claims(old.ClientID, a.nonce, sub), nil), "new-access-secret", "new-refresh-secret", nil)
			})
			q := url.Values{"state": {a.state}, "code": {"code-secret"}}
			if mode == "client" {
				q.Set("client_id", "oaiapp_foreign")
			}
			if mode == "deleted" {
				if err := m.Delete(old.ID); err != nil {
					t.Fatal(err)
				}
			}
			if sendCallback(t, a, q) != 400 {
				t.Fatal("binding change accepted")
			}
			if _, err := waitFor(t, a); err == nil {
				t.Fatal("binding change completed")
			}
			if mode == "client" && p.calls.Load() != 0 {
				t.Fatal("foreign client exchanged")
			}
			got, err := m.Get(old.ID)
			if mode == "deleted" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatal("deleted credential resurrected")
				}
			} else if err != nil || got.RefreshToken != old.RefreshToken || got.Subject != old.Subject {
				t.Fatal("existing credential overwritten")
			}
		})
	}
}

func TestIDTokenValidation(t *testing.T) {
	cases := map[string]func(map[string]any){
		"issuer":        func(c map[string]any) { c["iss"] = "https://foreign.example" },
		"audience":      func(c map[string]any) { c["aud"] = []string{"oaiapp_foreign"} },
		"expiration":    func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() },
		"missing exp":   func(c map[string]any) { delete(c, "exp") },
		"nonce":         func(c map[string]any) { c["nonce"] = "other-nonce" },
		"missing nonce": func(c map[string]any) { delete(c, "nonce") },
		"missing sub":   func(c map[string]any) { delete(c, "sub") },
		"empty sub":     func(c map[string]any) { c["sub"] = " " },
		"missing iat":   func(c map[string]any) { delete(c, "iat") },
		"future iat":    func(c map[string]any) { c["iat"] = time.Now().Add(time.Minute).Unix() },
		"not yet valid": func(c map[string]any) { c["nbf"] = time.Now().Add(time.Minute).Unix() },
		"wrong azp":     func(c map[string]any) { c["azp"] = "oaiapp_foreign" },
		"multi aud":     func(c map[string]any) { c["aud"] = []string{"oaiapp_new", "other"} },
		"empty sid":     func(c map[string]any) { c["sid"] = " " },
		"token hash":    func(c map[string]any) { c["at_hash"] = "bad-hash" },
		"signature":     func(c map[string]any) {},
		"unsigned":      func(c map[string]any) {},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			a := begin(t, m, "")
			claims := p.claims("oaiapp_new", a.nonce, "subject")
			mutate(claims)
			var key *ecdsa.PrivateKey
			if name == "signature" {
				key, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			}
			id := p.sign(claims, key)
			if name == "unsigned" {
				data, _ := json.Marshal(claims)
				id = "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(data) + "."
			}
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				tokenReply(w, id, "access-secret", "refresh-secret", nil)
			})
			sendCallback(t, a, url.Values{"state": {a.state}, "code": {"code"}, "client_id": {"oaiapp_new"}})
			if _, err := waitFor(t, a); err == nil {
				t.Fatal("invalid ID token accepted")
			}
			entries, _ := os.ReadDir(filepath.Join(m.dir, "accounts"))
			if len(entries) != 0 {
				t.Fatal("invalid identity persisted")
			}
		})
	}
}

func TestRequiredGrantedScopes(t *testing.T) {
	for _, missing := range []string{"openid", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"} {
		t.Run(missing, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			a := begin(t, m, "")
			scopes := strings.TrimSpace(strings.Replace(scopeList, missing, "", 1))
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				tokenReply(w, p.sign(p.claims("oaiapp_new", a.nonce, "subject"), nil), "access-secret", "refresh-secret", map[string]any{"scope": scopes})
			})
			// The callback's scope must never override the actual token grant.
			sendCallback(t, a, url.Values{"state": {a.state}, "code": {"code"}, "client_id": {"oaiapp_new"}, "scope": {scopeList}})
			if _, err := waitFor(t, a); err == nil {
				t.Fatal("missing required scope accepted")
			}
		})
	}
}

func TestAttemptCancellationExpiryAndWait(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	a := begin(t, m, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Wait(ctx); !errors.Is(err, context.Canceled) || a.Status().Status != "pending" {
		t.Fatal("wait cancellation stopped attempt")
	}
	a.Cancel()
	a.Cancel()
	if _, err := waitFor(t, a); !errors.Is(err, context.Canceled) || a.Status().Status != "failed" {
		t.Fatal("cancel did not finish attempt")
	}
	m.opts.attemptTTL = 20 * time.Millisecond
	b := begin(t, m, "")
	if _, err := waitFor(t, b); !errors.Is(err, context.DeadlineExceeded) || b.Status().Status != "expired" {
		t.Fatal("expiry did not finish attempt", err)
	}
	if _, err := m.Start(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatal("started with cancelled context")
	}
}

func TestRefreshRotationConcurrentManagers(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	old := seed(t, m)
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("client_id") != old.ClientID || r.Form.Get("refresh_token") != old.RefreshToken || r.Form.Has("scope") {
			t.Error("incorrect rotating refresh request")
		}
		time.Sleep(50 * time.Millisecond)
		tokenReply(w, "", "rotated-access-secret", "rotated-refresh-secret", map[string]any{"earliest_refresh_at": time.Now().Add(30 * time.Minute).Format(time.RFC3339)})
	})
	var managers []*Manager
	for range 8 {
		other, err := newManager(m.dir, p.options())
		if err != nil {
			t.Fatal(err)
		}
		managers = append(managers, other)
	}
	var wg sync.WaitGroup
	for _, manager := range managers {
		wg.Go(func() {
			token, err := manager.AccessToken(context.Background(), old.ID)
			if err != nil || token != "rotated-access-secret" {
				t.Error("refresh failed", err)
			}
		})
	}
	wg.Wait()
	if p.calls.Load() != 1 {
		t.Fatal("rotating token refreshed concurrently", p.calls.Load())
	}
	got, err := m.Get(old.ID)
	if err != nil || got.RefreshToken != "rotated-refresh-secret" || got.IDToken != old.IDToken || !got.ExpiresAt.After(time.Now()) {
		t.Fatal("refresh not atomically persisted", err)
	}
}

// Subprocesses use the same public store but a loopback-only test token endpoint.
func TestRefreshProcessHelper(t *testing.T) {
	if os.Getenv("MYAAW_CHATGPT_TEST_CHILD") != "1" {
		return
	}
	o := defaultOptions()
	o.tokenURL = os.Getenv("MYAAW_CHATGPT_TEST_TOKEN_URL")
	m, err := newManager(os.Getenv("MYAAW_CHATGPT_TEST_DIR"), o)
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.AccessToken(context.Background(), os.Getenv("MYAAW_CHATGPT_TEST_ID"))
	if err != nil || token != "process-access-secret" {
		t.Fatal("child refresh failed", err)
	}
}

func TestRefreshSerializedAcrossProcesses(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	old := seed(t, m)
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		if r.Form.Get("refresh_token") != old.RefreshToken {
			t.Error("unexpected refresh token")
		}
		time.Sleep(150 * time.Millisecond)
		tokenReply(w, "", "process-access-secret", "process-refresh-secret", nil)
	})
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestRefreshProcessHelper$")
			cmd.Env = append(os.Environ(), "MYAAW_CHATGPT_TEST_CHILD=1", "MYAAW_CHATGPT_TEST_TOKEN_URL="+p.options().tokenURL, "MYAAW_CHATGPT_TEST_DIR="+m.dir, "MYAAW_CHATGPT_TEST_ID="+old.ID)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("child failed: %v %s", err, out)
			}
		})
	}
	wg.Wait()
	if p.calls.Load() != 1 {
		t.Fatal("independent processes raced rotating refresh", p.calls.Load())
	}
}

func TestEarliestRefreshAndExpiry(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	old := seed(t, m)
	old.ExpiresAt = time.Now().Add(30 * time.Second)
	old.EarliestRefreshAt = time.Now().Add(20 * time.Second)
	if err := atomicJSON(m.accountPath(old.ID), old); err != nil {
		t.Fatal(err)
	}
	if token, err := m.AccessToken(context.Background(), old.ID); err != nil || token != old.AccessToken {
		t.Fatal("refreshed before earliest_refresh_at", err)
	}
	old.ExpiresAt = time.Now().Add(-time.Second)
	if err := atomicJSON(m.accountPath(old.ID), old); err != nil {
		t.Fatal(err)
	}
	if token, err := m.AccessToken(context.Background(), old.ID); err == nil || token != "" || p.calls.Load() != 0 {
		t.Fatal("used expired token or refreshed before permitted time")
	}
	for _, raw := range []string{`1790032532`, `"1790032532"`, `"2026-09-22T00:00:00Z"`, `null`, ``} {
		if _, err := parseEarliest(json.RawMessage(raw)); err != nil {
			t.Error("valid earliest refresh time rejected", raw)
		}
	}
	for _, raw := range []string{`-1`, `1790032532000`, `true`, `{}`, `"tomorrow"`} {
		if _, err := parseEarliest(json.RawMessage(raw)); err == nil {
			t.Error("invalid earliest refresh time accepted", raw)
		}
	}
}

func TestRefreshErrorsPreserveOrClearTokens(t *testing.T) {
	for _, code := range []string{"invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused", "invalid_client", "server_error", "untrusted-secret"} {
		t.Run(code, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			old := seed(t, m)
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": "refresh-secret"})
			})
			token, err := m.AccessToken(context.Background(), old.ID)
			if err == nil || token != "" || strings.Contains(err.Error(), "secret") {
				t.Fatal("unsafe refresh error", err)
			}
			got, err := m.Get(old.ID)
			if err != nil || got.ClientID != old.ClientID || got.Subject != old.Subject {
				t.Fatal("lost registration binding", err)
			}
			if terminalRefreshCode(code) {
				if got.AccessToken != "" || got.RefreshToken != "" || got.IDToken != "" {
					t.Fatal("unusable tokens retained")
				}
			} else if got.RefreshToken != old.RefreshToken {
				t.Fatal("recoverable error destroyed tokens")
			}
		})
	}
}

func TestStoreRejectsTraversalAndSymlinks(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	for _, id := range []string{"../host", "", "../../secrets", "HOST", "/tmp/anything"} {
		if _, err := m.Get(id); err == nil {
			t.Error("invalid ID accepted", id)
		}
		if err := m.Delete(id); err == nil {
			t.Error("invalid ID deleted", id)
		}
	}
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated Windows privileges")
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte(`{"id":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if err := os.Symlink(target, m.accountPath(id)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(id); err == nil {
		t.Fatal("symlink credential accepted")
	}
	linked := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(m.dir, linked); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(linked); err == nil {
		t.Fatal("symlink store accepted")
	}
}

func TestLockContextCancellation(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	unlock, err := m.lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := m.lock(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("contended lock ignored context", err)
	}
}

func TestTokenEndpointBodyIsNeverAnError(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	for _, response := range []struct {
		status int
		body   string
	}{
		{500, "access-secret refresh-secret"},
		{200, "invalid-json-secret"},
		{200, `{"expires_in":"secret","access_token":"secret"}`},
		{200, strings.Repeat("secret", 200000)},
	} {
		t.Run(fmt.Sprint(response.status, len(response.body)), func(t *testing.T) {
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(response.status)
				_, _ = io.WriteString(w, response.body)
			})
			old := seed(t, m)
			_, err := m.AccessToken(context.Background(), old.ID)
			if err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("token response exposed", err)
			}
		})
	}
}
