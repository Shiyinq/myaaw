package chatgpt

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AttemptStatus is the only representation suitable for status APIs. It never
// includes tokens, authorization URLs, codes, state, nonce, or PKCE material.
type AttemptStatus struct {
	Status       string `json:"status"`
	CredentialID string `json:"credential_id"`
	Email        string `json:"email"`
	Error        string `json:"error"`
}

// Attempt represents a one-shot sign-in. Its exported fields are immutable.
type Attempt struct {
	ID               string
	AuthorizationURL string // May contain id_token_hint; do not log this URL.
	ExpiresAt        time.Time

	mu           sync.Mutex
	status       AttemptStatus
	result       Credential
	err          error
	claimed      bool
	done         chan struct{}
	ctx          context.Context
	cancel       context.CancelFunc
	manager      *Manager
	previous     *record
	registration *registration
	state        string
	nonce        string
	verifier     string
	redirect     string
}

func randomValue() string {
	var value [32]byte
	_, _ = rand.Read(value[:])
	return base64.RawURLEncoding.EncodeToString(value[:])
}

// Start listens before returning the authorization URL. An empty credentialID
// creates a new registration; a nonempty ID reauthorizes exactly that binding.
// The supplied context owns the attempt's lifetime, not just this call.
func (m *Manager) Start(ctx context.Context, credentialID string) (*Attempt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var previous *record
	var pending *registration
	if credentialID != "" {
		unlock, err := m.lock(ctx)
		if err != nil {
			return nil, err
		}
		r, err := m.load(credentialID)
		if errors.Is(err, ErrNotFound) {
			var registration registration
			registration, err = m.loadRegistration(credentialID)
			if err == nil {
				pending = &registration
			}
		} else if err == nil {
			previous = &r
		}
		unlock()
		if err != nil {
			return nil, err
		}
	} else {
		credentialID = uuid.NewString()
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, errors.New("cannot listen for ChatGPT sign-in callback")
	}
	attemptCtx, cancel := context.WithTimeout(ctx, m.opts.attemptTTL)
	expires, _ := attemptCtx.Deadline()
	a := &Attempt{
		ID: uuid.NewString(), ExpiresAt: expires,
		status: AttemptStatus{Status: "pending", CredentialID: credentialID},
		done:   make(chan struct{}), ctx: attemptCtx, cancel: cancel,
		manager: m, previous: previous, registration: pending,
		state: randomValue(), nonce: randomValue(), verifier: randomValue(),
		redirect: "http://" + listener.Addr().String() + "/auth/callback",
	}
	authorize, err := url.Parse(m.opts.authorizeURL)
	if err != nil {
		listener.Close()
		cancel()
		return nil, errors.New("invalid ChatGPT authorization endpoint")
	}
	challenge := sha256.Sum256([]byte(a.verifier))
	params := url.Values{
		"client_id": {"dynamic_agent_client"}, "agent_name_hint": {"Myaaw"},
		"ext_agent_host_id": {m.hostID}, "response_type": {"code"},
		"redirect_uri": {a.redirect}, "scope": {scopeList}, "resource": {resource},
		"state": {a.state}, "nonce": {a.nonce}, "code_challenge_method": {"S256"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])},
	}
	if pending != nil {
		params.Set("client_id", pending.ClientID)
		params.Del("agent_name_hint")
	}
	if previous != nil {
		params.Set("client_id", previous.ClientID)
		params.Del("agent_name_hint")
		if previous.IDToken != "" {
			params.Set("id_token_hint", previous.IDToken)
		}
		if previous.Email != "" {
			params.Set("login_hint", previous.Email)
		}
	}
	authorize.RawQuery = params.Encode()
	a.AuthorizationURL = authorize.String()
	server := &http.Server{
		Handler: http.HandlerFunc(a.callback), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10,
		ErrorLog: log.New(io.Discard, "", 0),
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.finish("failed", Credential{}, errors.New("ChatGPT callback listener stopped"))
		}
	}()
	go func() {
		select {
		case <-a.ctx.Done():
			status := "failed"
			if errors.Is(a.ctx.Err(), context.DeadlineExceeded) {
				status = "expired"
			}
			a.finish(status, Credential{}, a.ctx.Err())
		case <-a.done:
		}
		// Let the handler send its static response before closing its connection.
		shutdownCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = server.Shutdown(shutdownCtx)
		_ = server.Close()
	}()
	return a, nil
}

// Wait's context only stops this waiter. Use Cancel to stop the attempt.
func (a *Attempt) Wait(ctx context.Context) (Credential, error) {
	select {
	case <-a.done:
		a.mu.Lock()
		defer a.mu.Unlock()
		result := a.result
		result.Scopes = append([]string(nil), result.Scopes...)
		return result, a.err
	case <-ctx.Done():
		return Credential{}, ctx.Err()
	}
}

func (a *Attempt) Status() AttemptStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *Attempt) Cancel() { a.finish("failed", Credential{}, context.Canceled) }

func (a *Attempt) finish(status string, result Credential, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.finishLocked(status, result, err)
}

func (a *Attempt) finishLocked(status string, result Credential, err error) {
	if a.status.Status != "pending" {
		return
	}
	if err != nil && errors.Is(a.ctx.Err(), context.DeadlineExceeded) {
		status, err = "expired", context.DeadlineExceeded
	}
	a.status.Status, a.result, a.err = status, result, err
	if err != nil {
		a.status.Error = err.Error()
	} else {
		a.status.CredentialID, a.status.Email = result.ID, result.Email
	}
	close(a.done)
	a.cancel()
}

func callbackReply(w http.ResponseWriter, success bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if success {
		_, _ = io.WriteString(w, "ChatGPT connected. You may close this window.\n")
	} else {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "ChatGPT sign-in could not be completed. Return to Myaaw and try again.\n")
	}
}

func (a *Attempt) callback(w http.ResponseWriter, r *http.Request) {
	expected, _ := url.Parse(a.redirect)
	if r.Method != http.MethodGet || r.URL.Path != "/auth/callback" || r.Host != expected.Host {
		callbackReply(w, false)
		return
	}
	q, parseErr := url.ParseQuery(r.URL.RawQuery)
	a.mu.Lock()
	if a.status.Status != "pending" || a.claimed {
		a.mu.Unlock()
		callbackReply(w, false)
		return
	}
	// Unauthenticated traffic must not consume or cancel a real sign-in.
	if parseErr != nil || len(r.URL.RawQuery) > 16<<10 || len(q["state"]) != 1 || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(a.state)) != 1 {
		a.mu.Unlock()
		callbackReply(w, false)
		return
	}
	a.claimed = true
	err := a.ctx.Err()
	for _, key := range []string{"code", "client_id", "error"} {
		if len(q[key]) > 1 {
			err = errors.New("ambiguous ChatGPT callback parameters")
		}
	}
	if err == nil && q.Has("error") {
		err = errors.New("ChatGPT authorization was not granted")
	}
	if err == nil && q.Get("code") == "" {
		err = errors.New("ChatGPT callback lacks authorization code")
	}
	clientID := q.Get("client_id")
	if err == nil {
		expectedClient := ""
		if a.previous != nil {
			expectedClient = a.previous.ClientID
		} else if a.registration != nil {
			expectedClient = a.registration.ClientID
		}
		if expectedClient == "" {
			if !issuedClient.MatchString(clientID) {
				err = errors.New("ChatGPT registration lacks a valid issued client ID")
			}
		} else if q.Has("client_id") && clientID != expectedClient {
			err = errors.New("ChatGPT reauthorization client ID mismatch")
		} else {
			clientID = expectedClient
		}
	}
	a.mu.Unlock()
	if err == nil {
		err = a.exchange(q.Get("code"), clientID)
	}
	if err != nil {
		a.finish("failed", Credential{}, err)
	}
	callbackReply(w, err == nil)
}

// Retain only the callback-issued client, never an unverified token or identity.
func (a *Attempt) retainRegistration(clientID string) error {
	if a.previous != nil {
		return nil
	}
	m := a.manager
	unlock, err := m.lock(a.ctx)
	if err != nil {
		return err
	}
	defer unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status.Status != "pending" || a.ctx.Err() != nil {
		return context.Canceled
	}
	if a.registration != nil {
		current, err := m.loadRegistration(a.registration.ID)
		if err != nil {
			return err
		}
		if current != *a.registration {
			return errors.New("ChatGPT pending registration changed during sign-in")
		}
		return nil
	}
	pending := registration{ID: a.status.CredentialID, ClientID: clientID, Issuer: m.opts.issuer, HostID: m.hostID}
	if _, err := m.registrations.Lstat(pending.ID + ".json"); !errors.Is(err, os.ErrNotExist) {
		return errors.New("ChatGPT pending registration already exists or cannot be created")
	}
	if err := atomicJSONAt(m.registrations, pending.ID+".json", pending); err != nil {
		return err
	}
	a.registration = &pending
	return nil
}

func (a *Attempt) exchange(code, clientID string) error {
	m := a.manager
	if err := a.retainRegistration(clientID); err != nil {
		return err
	}
	tokens, started, err := m.requestTokens(a.ctx, url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID},
		"code": {code}, "code_verifier": {a.verifier}, "redirect_uri": {a.redirect},
	})
	if err != nil {
		return err
	}
	claims, err := m.verifyIdentity(a.ctx, tokens.IDToken, tokens.AccessToken, clientID, a.nonce, true)
	if err != nil {
		return err
	}
	if a.previous != nil && claims.Subject != a.previous.Subject {
		return errors.New("ChatGPT reauthorization subject mismatch")
	}
	r, err := applyTokens(record{Credential: Credential{
		ID: a.Status().CredentialID, ClientID: clientID, Subject: claims.Subject, Email: claims.Email,
	}, Nonce: a.nonce}, tokens, started, true)
	if err != nil {
		return err
	}
	unlock, err := m.lock(a.ctx)
	if err != nil {
		return err
	}
	defer unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status.Status != "pending" || a.ctx.Err() != nil {
		return context.Canceled
	}
	if a.previous != nil {
		current, err := m.load(a.previous.ID)
		if err != nil {
			return err
		}
		if current.ClientID != clientID || current.Subject != claims.Subject || current.Revision != a.previous.Revision || current.RefreshToken != a.previous.RefreshToken || current.IDToken != a.previous.IDToken {
			return errors.New("ChatGPT registration changed during sign-in")
		}
	} else {
		if _, err := m.accounts.Lstat(r.ID + ".json"); !errors.Is(err, os.ErrNotExist) {
			return errors.New("ChatGPT registration already exists or cannot be created")
		}
		current, err := m.loadRegistration(r.ID)
		if err != nil {
			return err
		}
		if a.registration == nil || current != *a.registration {
			return errors.New("ChatGPT pending registration changed during sign-in")
		}
	}
	if err := m.save(r); err != nil {
		return err
	}
	if a.registration != nil {
		if err := m.registrations.Remove(r.ID + ".json"); err != nil {
			return errors.New("cannot remove completed ChatGPT pending registration")
		}
		if err := syncRoot(m.registrations); err != nil {
			return err
		}
	}
	// Retain a verified sign-in without plan permission, but never enable inference.
	if err := requireScopes(r.Scopes); err != nil {
		a.finishLocked("failed", Credential{}, err)
		return err
	}
	a.finishLocked("connected", r.Credential, nil)
	return nil
}
