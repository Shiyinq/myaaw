package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"myaaw/internal/agentdir"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultUsername and DefaultPassword are seeded on first run. They can be
	// changed after login through the auth endpoints.
	DefaultUsername = "myaaw"
	DefaultPassword = "myaawadmin"

	credentialsFileName = "controlplane.json"
	sessionTTL          = 24 * time.Hour
	minPasswordLength   = 8
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrWeakPassword       = fmt.Errorf("password must be at least %d characters", minPasswordLength)
)

type credentials struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	CreatedAt    string `json:"created_at"`
}

// AuthService stores the single control-plane user and its active sessions.
type AuthService struct {
	mu       sync.Mutex
	path     string
	creds    credentials
	sessions map[string]time.Time
	now      func() time.Time
}

// NewAuthService loads (or creates) credentials under ~/.myaaw.
func NewAuthService() (*AuthService, error) {
	root, err := agentdir.MyaawRoot()
	if err != nil {
		return nil, err
	}
	return NewAuthServiceAt(filepath.Join(root, credentialsFileName))
}

// NewAuthServiceAt is the testable constructor.
func NewAuthServiceAt(path string) (*AuthService, error) {
	s := &AuthService{
		path:     path,
		sessions: make(map[string]time.Time),
		now:      time.Now,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *AuthService) Username() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds.Username
}

// Login validates credentials and returns a new session token.
func (s *AuthService) Login(username, password string) (string, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if username != s.creds.Username {
		return "", time.Time{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(s.creds.PasswordHash), []byte(password)); err != nil {
		return "", time.Time{}, ErrInvalidCredentials
	}

	token, err := newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := s.now().Add(sessionTTL)
	s.sessions[token] = expiresAt
	return token, expiresAt, nil
}

func (s *AuthService) Validate(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	expiresAt, ok := s.sessions[token]
	if !ok {
		return false
	}
	if s.now().After(expiresAt) {
		delete(s.sessions, token)
		return false
	}
	return true
}

func (s *AuthService) Logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *AuthService) ChangePassword(currentPassword, newPassword string) error {
	if len(newPassword) < minPasswordLength {
		return ErrWeakPassword
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := bcrypt.CompareHashAndPassword([]byte(s.creds.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.creds.PasswordHash = string(hash)
	s.sessions = make(map[string]time.Time)
	return s.saveLocked()
}

func (s *AuthService) ChangeUsername(password, newUsername string) error {
	newUsername = strings.TrimSpace(newUsername)
	if newUsername == "" {
		return errors.New("username must not be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := bcrypt.CompareHashAndPassword([]byte(s.creds.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	s.creds.Username = newUsername
	return s.saveLocked()
}

func (s *AuthService) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return s.seed()
	}

	if err := json.Unmarshal(data, &s.creds); err != nil {
		return fmt.Errorf("parse %s: %w", s.path, err)
	}
	if s.creds.Username == "" || s.creds.PasswordHash == "" {
		return fmt.Errorf("%s is missing username or password_hash", s.path)
	}
	return nil
}

func (s *AuthService) seed() error {
	hash, err := bcrypt.GenerateFromPassword([]byte(DefaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	s.creds = credentials{
		Username:     DefaultUsername,
		PasswordHash: string(hash),
		CreatedAt:    s.now().UTC().Format(time.RFC3339),
	}
	return s.saveLocked()
}

func (s *AuthService) saveLocked() error {
	data, err := json.MarshalIndent(s.creds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
