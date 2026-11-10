package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"myaaw/internal/agentdir"
	"myaaw/internal/auth/chatgpt"
	"myaaw/internal/config"
)

// ProviderConnection is deliberately metadata-only. Reading it never refreshes
// tokens or claims to detect remote revocation before OpenAI reports it.
type ProviderConnection struct {
	Configured   bool       `json:"configured"`
	Status       string     `json:"status"`
	AccountLabel string     `json:"account_label"`
	ExpiresAt    *time.Time `json:"expires_at"`
}

func providerView(name string, p config.ProviderConfig) ProviderView {
	view := ProviderView{Name: name, Type: p.Type, BaseURL: p.BaseURL,
		DefaultModel: p.DefaultModel, APIKey: secretState(p.APIKey),
		AuthMode: p.AuthMode, CredentialID: p.CredentialID,
		ThinkingLevel: p.ThinkingLevel}
	if p.Type != chatgpt.ProviderType {
		return view
	}
	view.APIKey = SecretState{}
	view.AuthMode = "oauth"
	connection := &ProviderConnection{Status: "disconnected"}
	view.Connection = connection
	if p.CredentialID == "" {
		return view
	}
	connection.Status = "unavailable"
	manager, err := chatgpt.DefaultManager()
	if err != nil {
		return view
	}
	credential, err := manager.Get(p.CredentialID)
	if err != nil {
		return view
	}
	connection.Configured = true
	connection.AccountLabel = credential.Email
	if !credential.ExpiresAt.IsZero() {
		expires := credential.ExpiresAt
		connection.ExpiresAt = &expires
	}
	connection.Status = "connected"
	granted := make(map[string]bool)
	for _, scope := range credential.Scopes {
		granted[scope] = true
	}
	if credential.AccessToken == "" || credential.RefreshToken == "" ||
		!granted["openid"] || !granted["offline_access"] ||
		!granted["resource.invoke"] || !granted["chatgpt.tokens.use.direct"] {
		connection.Status = "reauthorization_required"
	} else if !credential.ExpiresAt.After(time.Now()) {
		connection.Status = "refresh_required"
	}
	return view
}

var (
	ErrChatGPTSessionNotFound = errors.New("ChatGPT Subscription session not found")
	ErrChatGPTSessionLimit    = errors.New("too many ChatGPT Subscription sign-in sessions; cancel a session or wait for expiry")
)

// ChatGPTAttemptStatus is the auth package's metadata-only status contract.
type ChatGPTAttemptStatus = chatgpt.AttemptStatus

type ChatGPTConnectView struct {
	SessionID        string    `json:"session_id"`
	AuthorizationURL string    `json:"authorization_url"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type chatGPTSession struct {
	agent   string
	attempt *chatgpt.Attempt
}

type chatGPTSessions struct {
	mu      sync.Mutex
	entries map[string]chatGPTSession
}

// StartChatGPT uses an attempt-owned context: an HTTP response ending must not
// close the callback listener. Both retained status and listener lifetimes are
// bounded; statuses remain briefly available after timeout for the polling UI.
func (s *ConfigService) StartChatGPT(agent, credentialID string) (ChatGPTConnectView, error) {
	// The new-agent wizard signs in before it persists the agent. Validating
	// the name scopes that session without creating any agent files.
	if err := agentdir.ValidateName(agent); err != nil {
		return ChatGPTConnectView{}, ErrAgentNotFound
	}
	s.chatGPT.mu.Lock()
	defer s.chatGPT.mu.Unlock()
	if s.chatGPT.entries == nil {
		s.chatGPT.entries = make(map[string]chatGPTSession)
	}
	now := time.Now()
	for id, session := range s.chatGPT.entries {
		if now.After(session.attempt.ExpiresAt.Add(2 * time.Minute)) {
			session.attempt.Cancel()
			delete(s.chatGPT.entries, id)
		}
	}
	if len(s.chatGPT.entries) >= 32 {
		return ChatGPTConnectView{}, ErrChatGPTSessionLimit
	}
	for _, session := range s.chatGPT.entries {
		if session.agent == agent && session.attempt.Status().Status == "pending" {
			return ChatGPTConnectView{}, ErrChatGPTSessionLimit
		}
	}
	manager, err := chatgpt.DefaultManager()
	if err != nil {
		return ChatGPTConnectView{}, fmt.Errorf("%w: cannot open ChatGPT Subscription credentials", ErrInvalidConfig)
	}
	// Do not preflight Get: Start also accepts retained pending registrations.
	attempt, err := manager.Start(context.Background(), credentialID)
	if err != nil {
		return ChatGPTConnectView{}, fmt.Errorf("%w: cannot start ChatGPT Subscription sign-in; check the local credential_id and callback listener", ErrInvalidConfig)
	}
	s.chatGPT.entries[attempt.ID] = chatGPTSession{agent: agent, attempt: attempt}
	time.AfterFunc(time.Until(attempt.ExpiresAt.Add(2*time.Minute)), func() {
		s.chatGPT.mu.Lock()
		defer s.chatGPT.mu.Unlock()
		attempt.Cancel()
		delete(s.chatGPT.entries, attempt.ID)
	})
	return ChatGPTConnectView{SessionID: attempt.ID, AuthorizationURL: attempt.AuthorizationURL, ExpiresAt: attempt.ExpiresAt}, nil
}

func (s *ConfigService) ChatGPTStatus(agent, id string, cancel bool) (chatgpt.AttemptStatus, error) {
	if err := agentdir.ValidateName(agent); err != nil {
		return chatgpt.AttemptStatus{}, ErrAgentNotFound
	}
	s.chatGPT.mu.Lock()
	defer s.chatGPT.mu.Unlock()
	session, ok := s.chatGPT.entries[id]
	if !ok || session.agent != agent || time.Now().After(session.attempt.ExpiresAt.Add(2*time.Minute)) {
		return chatgpt.AttemptStatus{}, ErrChatGPTSessionNotFound
	}
	if cancel {
		session.attempt.Cancel()
	}
	return session.attempt.Status(), nil
}

// DisconnectProvider detaches this reference, not the shared host credential.
// It is neither credential deletion nor remote revocation.
func (s *ConfigService) DisconnectProvider(agent, id string) error {
	cfg, err := loadAgentConfig(agent)
	if err != nil {
		return err
	}
	p, ok := cfg.Providers[id]
	if !ok {
		return ErrProviderNotFound
	}
	if p.Type != chatgpt.ProviderType {
		return fmt.Errorf("%w: disconnect is only for ChatGPT Subscription", ErrInvalidConfig)
	}
	p.CredentialID, p.APIKey, p.BaseURL = "", "", ""
	p.AuthMode = "oauth"
	cfg.Providers[id] = p
	return saveAgentConfig(agent, cfg)
}
