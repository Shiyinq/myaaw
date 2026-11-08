package chatgpt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFailedRegistrationReusesIssuedClientAfterRestart(t *testing.T) {
	for _, failure := range []string{"invalid_grant", "temporary error", "invalid identity", "incomplete response"} {
		t.Run(failure, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			other := seed(t, m)
			a := begin(t, m, "")
			id := a.Status().CredentialID
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				switch failure {
				case "invalid_grant":
					w.WriteHeader(400)
					_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"code-secret"}`)
				case "temporary error":
					w.WriteHeader(503)
				case "invalid identity":
					tokenReply(w, p.sign(p.claims("oaiapp_registered", "wrong-nonce", "unverified-subject"), nil), "access-secret", "refresh-secret", nil)
				case "incomplete response":
					_, _ = io.WriteString(w, `{}`)
				}
			})
			if sendCallback(t, a, url.Values{"state": {a.state}, "code": {"code-secret"}, "client_id": {"oaiapp_registered"}}) != 400 {
				t.Fatal("failed exchange accepted")
			}
			if _, err := waitFor(t, a); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("failed exchange succeeded or leaked data", err)
			}
			if _, err := m.Get(id); !errors.Is(err, ErrNotFound) {
				t.Fatal("unverified registration exposed as an account", err)
			}
			if token, err := m.AccessToken(context.Background(), id); token != "" || !errors.Is(err, ErrNotFound) {
				t.Fatal("unverified registration enabled inference", err)
			}
			data, err := os.ReadFile(filepath.Join(m.dir, "registrations", id+".json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"secret", "unverified-subject", "nonce", "verifier", "access_token", "refresh_token", "id_token"} {
				if strings.Contains(string(data), forbidden) {
					t.Fatal("pending recovery record retained unverified identity or secrets")
				}
			}
			restarted, err := newManager(m.dir, p.options())
			if err != nil {
				t.Fatal(err)
			}
			b := begin(t, restarted, id)
			q := authParams(t, b)
			if q.Get("client_id") != "oaiapp_registered" || q.Has("agent_name_hint") || q.Has("id_token_hint") || q.Has("login_hint") || q.Get("ext_agent_host_id") != m.hostID {
				t.Fatal("retry repeated dynamic registration or used unverified hints")
			}
			if b.state == a.state || b.nonce == a.nonce || b.verifier == a.verifier {
				t.Fatal("retry reused authorization proof material")
			}
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				if r.Form.Get("client_id") != "oaiapp_registered" || r.Form.Get("code") != "fresh-code" {
					t.Error("retry did not exchange a fresh code with the retained client")
				}
				tokenReply(w, p.sign(p.claims("oaiapp_registered", b.nonce, "verified-subject"), nil), "new-access-secret", "new-refresh-secret", nil)
			})
			if sendCallback(t, b, url.Values{"state": {b.state}, "code": {"fresh-code"}}) != 200 {
				t.Fatal("retry callback rejected")
			}
			got, err := waitFor(t, b)
			if err != nil || got.ID != id || got.Subject != "verified-subject" {
				t.Fatal("retry did not finish the original isolated registration", err)
			}
			if _, err := m.loadRegistration(id); !errors.Is(err, ErrNotFound) {
				t.Fatal("completed pending registration not removed", err)
			}
			unchanged, err := m.Get(other.ID)
			if err != nil || unchanged.RefreshToken != other.RefreshToken || unchanged.Subject != other.Subject {
				t.Fatal("new registration altered an existing account", err)
			}
		})
	}
}

func TestPendingRegistrationCannotChangeClientOrSurviveDelete(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	a := begin(t, m, "")
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	})
	sendCallback(t, a, url.Values{"state": {a.state}, "code": {"code"}, "client_id": {"oaiapp_pending"}})
	_, _ = waitFor(t, a)
	id := a.Status().CredentialID
	b := begin(t, m, id)
	sendCallback(t, b, url.Values{"state": {b.state}, "code": {"code"}, "client_id": {"oaiapp_foreign"}})
	if _, err := waitFor(t, b); err == nil || p.calls.Load() != 1 {
		t.Fatal("pending retry accepted a foreign client")
	}
	c := begin(t, m, id)
	if err := m.Delete(id); err != nil {
		t.Fatal(err)
	}
	// A callback to a deleted pending registration must not even exchange its code.
	sendCallback(t, c, url.Values{"state": {c.state}, "code": {"code"}})
	if _, err := waitFor(t, c); err == nil || p.calls.Load() != 1 {
		t.Fatal("deleted pending registration exchanged a code")
	}
	if _, err := m.Start(context.Background(), id); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted registration was resurrected", err)
	}
}

func TestUntrustedStateDoesNotPreventValidCallback(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	a := begin(t, m, "")
	for _, query := range []string{"state=wrong&error=access_denied", "code=secret", "state=%zz", "state=" + a.state + "&state=" + a.state} {
		w := httptest.NewRecorder()
		a.callback(w, httptest.NewRequest(http.MethodGet, a.redirect+"?"+query, nil))
		if w.Code != 400 || a.Status().Status != "pending" || p.calls.Load() != 0 {
			t.Fatal("untrusted callback consumed sign-in")
		}
	}
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		tokenReply(w, p.sign(p.claims("oaiapp_valid", a.nonce, "subject"), nil), "access-secret", "refresh-secret", nil)
	})
	if sendCallback(t, a, url.Values{"state": {a.state}, "code": {"code"}, "client_id": {"oaiapp_valid"}}) != 200 {
		t.Fatal("valid callback rejected after unrelated traffic")
	}
	if _, err := waitFor(t, a); err != nil {
		t.Fatal(err)
	}
}

func TestReauthorizationCannotOverwriteNewerSignInOrRotation(t *testing.T) {
	for _, update := range []string{"reauthorization", "refresh"} {
		t.Run(update, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			old := seed(t, m)
			stale := begin(t, m, old.ID)
			if update == "reauthorization" {
				fresh := begin(t, m, old.ID)
				p.setHandler(func(w http.ResponseWriter, r *http.Request) {
					tokenReply(w, p.sign(p.claims(old.ClientID, fresh.nonce, old.Subject), nil), "latest-access-secret", "latest-refresh-secret", nil)
				})
				sendCallback(t, fresh, url.Values{"state": {fresh.state}, "code": {"fresh-code"}})
				if _, err := waitFor(t, fresh); err != nil {
					t.Fatal(err)
				}
			} else {
				p.setHandler(func(w http.ResponseWriter, r *http.Request) {
					tokenReply(w, "", "latest-access-secret", "latest-refresh-secret", nil)
				})
				if _, err := m.AccessToken(context.Background(), old.ID); err != nil {
					t.Fatal(err)
				}
			}
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				tokenReply(w, p.sign(p.claims(old.ClientID, stale.nonce, old.Subject), nil), "stale-access-secret", "stale-refresh-secret", nil)
			})
			if sendCallback(t, stale, url.Values{"state": {stale.state}, "code": {"stale-code"}}) != 400 {
				t.Fatal("stale sign-in overwrote a newer session")
			}
			if _, err := waitFor(t, stale); err == nil {
				t.Fatal("stale sign-in succeeded")
			}
			got, err := m.Get(old.ID)
			if err != nil || got.RefreshToken != "latest-refresh-secret" {
				t.Fatal("newer tokens lost", err)
			}
		})
	}
}

func TestVerifiedSignInWithoutPlanPermissionIsRetainedButNotUsable(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	a := begin(t, m, "")
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		tokenReply(w, p.sign(p.claims("oaiapp_disabled", a.nonce, "subject"), nil), "access-secret", "refresh-secret", map[string]any{"scope": "openid profile email offline_access"})
	})
	sendCallback(t, a, url.Values{"state": {a.state}, "code": {"code"}, "client_id": {"oaiapp_disabled"}, "scope": {scopeList}})
	if _, err := waitFor(t, a); err == nil || a.Status().Status != "failed" {
		t.Fatal("plan-disabled grant enabled sign-in for inference")
	}
	id := a.Status().CredentialID
	got, err := m.Get(id)
	if err != nil || got.Subject != "subject" || got.ClientID != "oaiapp_disabled" || got.IDToken == "" || requireScopes(got.Scopes) == nil {
		t.Fatal("verified disabled sign-in was not retained", err)
	}
	if token, err := m.AccessToken(context.Background(), id); token != "" || err == nil || p.calls.Load() != 1 {
		t.Fatal("disabled grant used a token or attempted refresh", err)
	}
	b := begin(t, m, id)
	q := authParams(t, b)
	if q.Get("client_id") != got.ClientID || q.Get("id_token_hint") != got.IDToken || q.Get("scope") != scopeList {
		t.Fatal("disabled grant cannot reauthorize the same account with full scopes")
	}
}

func TestRevokedRefreshErrorsAndRecoveryBinding(t *testing.T) {
	for _, status := range []int{400, 401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			old := seed(t, m)
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "refresh_token_invalidated", "message": "refresh-secret"}})
			})
			token, err := m.AccessToken(context.Background(), old.ID)
			terminal := status == 400 || status == 401 || status == 403
			if token != "" || err == nil || strings.Contains(err.Error(), "secret") || errors.Is(err, ErrReauthorizationRequired) != terminal {
				t.Fatal("revocation incorrectly classified or leaked data", err)
			}
			got, err := m.Get(old.ID)
			if err != nil || got.Subject != old.Subject || got.ClientID != old.ClientID || got.Email != old.Email {
				t.Fatal("revocation lost account binding", err)
			}
			if terminal {
				if got.AccessToken != "" || got.RefreshToken != "" || got.IDToken != "" || !got.ExpiresAt.IsZero() {
					t.Fatal("confirmed revocation retained usable tokens")
				}
				if _, err := m.AccessToken(context.Background(), old.ID); !errors.Is(err, ErrReauthorizationRequired) || p.calls.Load() != 1 {
					t.Fatal("confirmed revoked refresh token retried", err)
				}
				a := begin(t, m, old.ID)
				q := authParams(t, a)
				if q.Get("client_id") != old.ClientID || q.Get("login_hint") != old.Email || q.Has("id_token_hint") || q.Has("agent_name_hint") || q.Get("ext_agent_host_id") != m.hostID {
					t.Fatal("revocation retry lost registration or used cleared hints")
				}
			} else if got.RefreshToken != old.RefreshToken {
				t.Fatal("temporary failure destroyed renewable session")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b cancelOnClose) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func TestRefreshPersistsRotationWhenCallerCancelsAfterResponse(t *testing.T) {
	p := newTestIssuer(t)
	m := managerFor(t, p)
	old := seed(t, m)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.opts.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(r)
		if err == nil && r.URL.Path == "/api/accounts/oauth/token" {
			resp.Body = cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
		}
		return resp, err
	})
	p.setHandler(func(w http.ResponseWriter, r *http.Request) {
		tokenReply(w, p.sign(p.claims(old.ClientID, old.Nonce, old.Subject), nil), "rotated-access-secret", "rotated-refresh-secret", nil)
	})
	if token, err := m.AccessToken(ctx, old.ID); token != "" || !errors.Is(err, context.Canceled) {
		t.Fatal("refresh did not respect caller cancellation", err)
	}
	got, err := m.Get(old.ID)
	if err != nil || got.RefreshToken != "rotated-refresh-secret" || got.IDToken == old.IDToken || !got.ExpiresAt.After(time.Now()) {
		t.Fatal("cancellation discarded a completed and verified rotation", err)
	}
	if token, err := m.AccessToken(context.Background(), old.ID); err != nil || token != "rotated-access-secret" || p.calls.Load() != 1 {
		t.Fatal("completed rotation was retried", err)
	}
}

func TestPinnedDirectoriesPreventTokenRedirection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlink creation may require Windows privileges")
	}
	for _, directory := range []string{"accounts", "store"} {
		t.Run(directory, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			old := seed(t, m)
			target := t.TempDir()
			sentinel := filepath.Join(target, old.ID+".json")
			if err := os.WriteFile(sentinel, []byte("outside-secret"), 0o600); err != nil {
				t.Fatal(err)
			}
			path := m.dir
			if directory == "accounts" {
				path = filepath.Join(path, "accounts")
			}
			if err := os.Rename(path, path+"-original"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			got, err := m.Get(old.ID)
			if err != nil || got.RefreshToken != old.RefreshToken {
				t.Fatal("directory swap redirected token reads", err)
			}
			p.setHandler(func(w http.ResponseWriter, r *http.Request) {
				tokenReply(w, "", "pinned-access-secret", "pinned-refresh-secret", nil)
			})
			if _, err := m.AccessToken(context.Background(), old.ID); err != nil {
				t.Fatal("directory swap redirected atomic writes", err)
			}
			if err := m.Delete(old.ID); err != nil {
				t.Fatal("directory swap redirected deletion", err)
			}
			data, err := os.ReadFile(sentinel)
			if err != nil || string(data) != "outside-secret" {
				t.Fatal("directory swap modified an outside file", err)
			}
			if _, err := NewManager(m.dir); err == nil {
				t.Fatal("new manager followed a swapped directory symlink")
			}
		})
	}
}

func TestHardLinksRejectedBeforePermissionChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission assertion")
	}
	for _, file := range []string{"account", "host", "lock"} {
		t.Run(file, func(t *testing.T) {
			p := newTestIssuer(t)
			m := managerFor(t, p)
			old := seed(t, m)
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("outside-secret"), 0o644); err != nil {
				t.Fatal(err)
			}
			path := m.accountPath(old.ID)
			if file == "host" {
				path = filepath.Join(m.dir, "host.json")
			} else if file == "lock" {
				path = filepath.Join(m.dir, ".lock")
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(outside, path); err != nil {
				t.Fatal(err)
			}
			if file == "host" {
				if _, err := NewManager(m.dir); err == nil {
					t.Fatal("hard-linked host accepted")
				}
			} else if _, err := m.Get(old.ID); err == nil {
				t.Fatal("hard-linked credential or lock accepted")
			}
			info, err := os.Stat(outside)
			if err != nil || info.Mode().Perm() != 0o644 {
				t.Fatal("rejected link changed outside file permissions", err)
			}
		})
	}
}
