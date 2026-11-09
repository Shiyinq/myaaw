// Package chatgpt implements OpenAI's public-client ChatGPT Subscription OAuth
// flow. Credentials belong to the host, never to an individual Myaaw agent.
// Credential values and authorization URLs (which may contain ID-token hints)
// are secrets: callers must not log them or expose them in status responses.
package chatgpt

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"myaaw/internal/agentdir"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
)

const (
	ProviderType = "openai_chatgpt"
	Label        = "ChatGPT Subscription"

	issuerURL = "https://auth.openai.com"
	resource  = "https://api.openai.com/v1"
	scopeList = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
)

var (
	ErrReauthorizationRequired = errors.New("ChatGPT sign-in required")
	ErrNotFound                = os.ErrNotExist
	issuedClient               = regexp.MustCompile(`^oaiapp_[A-Za-z0-9_-]{1,240}$`)
)

// Credential is a single registration, not an email-keyed account. ID is an
// opaque local identifier; ClientID and Subject are the validated OAuth binding.
// Its token fields must never be included in API status responses.
type Credential struct {
	ID           string    `json:"id"`
	ClientID     string    `json:"client_id"`
	Subject      string    `json:"subject"`
	Email        string    `json:"email"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	IDToken      string    `json:"id_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       []string  `json:"scopes"`
}

type record struct {
	Credential
	EarliestRefreshAt time.Time `json:"earliest_refresh_at,omitempty"`
	Nonce             string    `json:"nonce,omitempty"`
	Revision          string    `json:"revision,omitempty"`
}

// A callback-issued client is not an account until an ID token is verified.
// Keep it separately so invalid_grant recovery never repeats dynamic registration.
type registration struct {
	ID       string `json:"id"`
	ClientID string `json:"client_id"`
	Issuer   string `json:"issuer"`
	HostID   string `json:"ext_agent_host_id"`
}

// Manager is safe for concurrent use, including alongside managers in other
// processes. All disk transactions and rotating refreshes share a kernel lock.
type Manager struct {
	dir           string
	hostID        string
	opts          options
	keys          oidc.KeySet
	store         *os.Root
	accounts      *os.Root
	registrations *os.Root
}

// Endpoint overrides deliberately remain package-private and are only for tests.
type options struct {
	authorizeURL string
	tokenURL     string
	issuer       string
	jwksURL      string
	client       *http.Client
	attemptTTL   time.Duration
}

func defaultOptions() options {
	return options{
		authorizeURL: issuerURL + "/api/accounts/authorize",
		tokenURL:     issuerURL + "/api/accounts/oauth/token",
		issuer:       issuerURL,
		jwksURL:      issuerURL + "/.well-known/jwks.json",
		client:       &http.Client{Timeout: 30 * time.Second},
		attemptTTL:   10 * time.Minute,
	}
}

func DefaultManager() (*Manager, error) {
	root, err := agentdir.MyaawRoot()
	if err != nil {
		return nil, errors.New("cannot locate Myaaw host credentials")
	}
	parent := filepath.Join(root, "credentials")
	if err := privateDir(parent); err != nil {
		return nil, err
	}
	return NewManager(filepath.Join(parent, "chatgpt"))
}

func NewManager(dir string) (*Manager, error) { return newManager(dir, defaultOptions()) }

func newManager(dir string, opts options) (*Manager, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("credential directory is required")
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.New("invalid credential directory")
	}
	if err := privateDir(absolute); err != nil {
		return nil, err
	}
	store, err := openPrivateDir(absolute)
	if err != nil {
		return nil, err
	}
	accounts, err := privateSubdir(store, "accounts")
	if err != nil {
		store.Close()
		return nil, err
	}
	registrations, err := privateSubdir(store, "registrations")
	if err != nil {
		accounts.Close()
		store.Close()
		return nil, err
	}
	initialized := false
	defer func() {
		if !initialized {
			registrations.Close()
			accounts.Close()
			store.Close()
		}
	}()
	// Never follow token-endpoint or JWKS redirects with credential-bearing data.
	client := *opts.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	opts.client = &client
	m := &Manager{dir: absolute, opts: opts, store: store, accounts: accounts, registrations: registrations}
	m.keys = oidc.NewRemoteKeySet(oidc.ClientContext(context.Background(), opts.client), opts.jwksURL)
	unlock, err := m.lock(context.Background())
	if err != nil {
		return nil, err
	}
	defer unlock()
	var host struct {
		ID string `json:"ext_agent_host_id"`
	}
	err = readJSONAt(m.store, "host.json", &host)
	if errors.Is(err, os.ErrNotExist) {
		host.ID = "urn:uuid:" + uuid.NewString()
		err = atomicJSONAt(m.store, "host.json", host)
	}
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(host.ID, "urn:uuid:") || !validID(strings.TrimPrefix(host.ID, "urn:uuid:")) {
		return nil, errors.New("invalid ChatGPT host identifier")
	}
	m.hostID = host.ID
	initialized = true
	return m, nil
}

func validID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}

func (m *Manager) load(id string) (record, error) {
	var r record
	if !validID(id) {
		return r, errors.New("invalid ChatGPT credential ID")
	}
	if err := readJSONAt(m.accounts, id+".json", &r); err != nil {
		return r, err
	}
	if r.ID != id || !issuedClient.MatchString(r.ClientID) || strings.TrimSpace(r.Subject) == "" {
		return record{}, errors.New("invalid ChatGPT credential record")
	}
	return r, nil
}

func (m *Manager) loadRegistration(id string) (registration, error) {
	var r registration
	if !validID(id) {
		return r, errors.New("invalid ChatGPT credential ID")
	}
	if err := readJSONAt(m.registrations, id+".json", &r); err != nil {
		return r, err
	}
	if r.ID != id || !issuedClient.MatchString(r.ClientID) || r.Issuer != m.opts.issuer || r.HostID != m.hostID {
		return registration{}, errors.New("invalid ChatGPT pending registration")
	}
	return r, nil
}

func (m *Manager) save(r record) error {
	r.Revision = uuid.NewString()
	return atomicJSONAt(m.accounts, r.ID+".json", r)
}

func (m *Manager) accountPath(id string) string {
	return filepath.Join(m.dir, "accounts", id+".json")
}

func (m *Manager) Get(id string) (Credential, error) {
	unlock, err := m.lock(context.Background())
	if err != nil {
		return Credential{}, err
	}
	defer unlock()
	r, err := m.load(id)
	return r.Credential, err
}

// Delete removes the local registration only. It is not remote session
// revocation; the user can disconnect the app in ChatGPT Settings.
func (m *Manager) Delete(id string) error {
	unlock, err := m.lock(context.Background())
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := m.load(id); err != nil {
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		if _, err := m.loadRegistration(id); err != nil {
			return err
		}
	} else {
		if err := m.accounts.Remove(id + ".json"); err != nil {
			return errors.New("cannot delete ChatGPT credential")
		}
		if err := syncRoot(m.accounts); err != nil {
			return err
		}
	}
	// Remove any pending recovery record too, so Delete cannot resurrect it.
	if err := m.registrations.Remove(id + ".json"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("cannot delete ChatGPT pending registration")
	}
	return syncRoot(m.registrations)
}

func requireScopes(scopes []string) error {
	granted := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		granted[s] = true
	}
	for _, s := range []string{"openid", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"} {
		if !granted[s] {
			return errors.New("ChatGPT grant lacks required identity, offline, or plan-usage permission")
		}
	}
	return nil
}

func (m *Manager) AccessToken(ctx context.Context, credentialID string) (string, error) {
	unlock, err := m.lock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()
	// Reload under the process-shared lock: a previous holder may have rotated.
	r, err := m.load(credentialID)
	if err != nil {
		return "", err
	}
	if r.AccessToken == "" || r.RefreshToken == "" {
		return "", ErrReauthorizationRequired
	}
	if err := requireScopes(r.Scopes); err != nil {
		return "", err
	}
	now := time.Now()
	if now.Add(time.Minute).Before(r.ExpiresAt) {
		return r.AccessToken, nil
	}
	if now.Before(r.EarliestRefreshAt) {
		if now.Before(r.ExpiresAt) {
			return r.AccessToken, nil
		}
		return "", errors.New("ChatGPT token expired before permitted refresh time")
	}
	updated, err := m.refresh(ctx, r)
	if err != nil {
		if errors.Is(err, ErrReauthorizationRequired) {
			// Retain the verified account/client binding and host ID for reauth.
			r.AccessToken, r.RefreshToken, r.IDToken = "", "", ""
			r.Nonce = ""
			r.ExpiresAt, r.EarliestRefreshAt = time.Time{}, time.Time{}
			if saveErr := m.save(r); saveErr != nil {
				return "", saveErr
			}
		}
		return "", err
	}
	if err := ctx.Err(); err != nil {
		// A successful rotation must still be persisted if the caller cancelled.
		if saveErr := m.save(updated); saveErr != nil {
			return "", saveErr
		}
		return "", err
	}
	if err := m.save(updated); err != nil {
		return "", err
	}
	if err := requireScopes(updated.Scopes); err != nil {
		return "", err
	}
	return updated.AccessToken, nil
}
