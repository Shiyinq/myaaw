package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultUsername, DefaultPassword, and DefaultRole are seeded on first run.
	// Credentials can be changed after login through the auth endpoints.
	DefaultUsername = "myaaw"
	DefaultPassword = "myaawadmin"
	DefaultRole     = "admin"

	sessionTTL        = 24 * time.Hour
	minPasswordLength = 8
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrWeakPassword       = fmt.Errorf("password must be at least %d characters", minPasswordLength)
)

type credentials struct {
	Username        string
	PasswordHash    string
	Role            string
	UserID          int
	PasswordChanged bool
	CreatedAt       string
	Settings        Settings
}

// AuthService stores the single control-plane user in controlplane.db and keeps
// active bearer sessions in memory.
type AuthService struct {
	mu       sync.Mutex
	db       *ControlPlaneDB
	ownsDB   bool
	creds    credentials
	sessions map[string]time.Time
	media    *MediaSigner
	now      func() time.Time
}

// NewAuthService opens ~/.myaaw/controlplane/controlplane.db and loads (or seeds)
// the control-plane account and settings.
func NewAuthService() (*AuthService, error) {
	cpdb, err := OpenControlPlaneDB()
	if err != nil {
		return nil, err
	}
	s, err := newAuthService(cpdb, true)
	if err != nil {
		_ = cpdb.Close()
		return nil, err
	}
	return s, nil
}

// NewAuthServiceWithDB initializes AuthService on a shared *ControlPlaneDB.
func NewAuthServiceWithDB(db *ControlPlaneDB) (*AuthService, error) {
	if db == nil {
		return nil, errors.New("controlplane db is required")
	}
	return newAuthService(db, false)
}

// NewAuthServiceAt opens a dedicated SQLite database at path (ideal for tests).
func NewAuthServiceAt(path string) (*AuthService, error) {
	cpdb, err := OpenControlPlaneDBAt(path)
	if err != nil {
		return nil, err
	}
	s, err := newAuthService(cpdb, true)
	if err != nil {
		_ = cpdb.Close()
		return nil, err
	}
	return s, nil
}

func newAuthService(cpdb *ControlPlaneDB, ownsDB bool) (*AuthService, error) {
	s := &AuthService{
		db:       cpdb,
		ownsDB:   ownsDB,
		sessions: make(map[string]time.Time),
		media:    newMediaSignerOrNil(),
		now:      time.Now,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close closes the underlying database when owned by this AuthService.
func (s *AuthService) Close() error {
	if s.ownsDB && s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *AuthService) Username() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds.Username
}

// Role returns the control-plane account role (defaults to "admin").
func (s *AuthService) Role() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creds.Role == "" {
		return DefaultRole
	}
	return s.creds.Role
}

// UserID returns the agent user ID associated with the control-plane login.
func (s *AuthService) UserID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds.UserID
}

// MustChangePassword reports whether the account still uses the seeded password.
func (s *AuthService) MustChangePassword() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.creds.PasswordChanged
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
	s.creds.PasswordChanged = true
	s.sessions = make(map[string]time.Time)
	return s.saveAccountLocked()
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
	return s.saveAccountLocked()
}

func (s *AuthService) load() error {
	if err := s.loadOrSeedAccount(); err != nil {
		return err
	}
	return s.loadOrSeedSettings()
}

func (s *AuthService) loadOrSeedAccount() error {
	var (
		username        string
		passwordHash    string
		role            string
		userID          int
		passwordChanged int
		createdAt       string
	)
	err := s.db.DB().QueryRow(
		`SELECT username, password_hash, role, user_id, password_changed, created_at FROM auth_account WHERE id = 1`,
	).Scan(&username, &passwordHash, &role, &userID, &passwordChanged, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s.seedAccount()
	}
	if err != nil {
		return fmt.Errorf("load auth_account: %w", err)
	}
	if username == "" || passwordHash == "" {
		return errors.New("controlplane auth_account is missing username or password_hash")
	}
	if strings.TrimSpace(role) == "" {
		role = DefaultRole
	}

	s.creds.Username = username
	s.creds.PasswordHash = passwordHash
	s.creds.Role = role
	s.creds.UserID = userID
	s.creds.PasswordChanged = passwordChanged != 0
	s.creds.CreatedAt = createdAt
	return nil
}

func (s *AuthService) seedAccount() error {
	hash, err := bcrypt.GenerateFromPassword([]byte(DefaultPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339)
	s.creds.Username = DefaultUsername
	s.creds.PasswordHash = string(hash)
	s.creds.Role = DefaultRole
	s.creds.UserID = 0
	s.creds.PasswordChanged = false
	s.creds.CreatedAt = now
	return s.saveAccountLocked()
}

func (s *AuthService) loadOrSeedSettings() error {
	var rawJSON string
	err := s.db.DB().QueryRow(`SELECT data_json FROM settings WHERE id = 1`).Scan(&rawJSON)
	if errors.Is(err, sql.ErrNoRows) {
		s.creds.Settings = DefaultSettings()
		return s.saveSettingsLocked()
	}
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	var patch SettingsPatch
	if err := json.Unmarshal([]byte(rawJSON), &patch); err != nil {
		return fmt.Errorf("parse settings: %w", err)
	}
	s.creds.Settings = DefaultSettings().Apply(patch)
	return nil
}

func (s *AuthService) saveAccountLocked() error {
	now := s.now().UTC().Format(time.RFC3339)
	createdAt := s.creds.CreatedAt
	if createdAt == "" {
		createdAt = now
		s.creds.CreatedAt = now
	}
	role := strings.TrimSpace(s.creds.Role)
	if role == "" {
		role = DefaultRole
		s.creds.Role = role
	}
	changed := 0
	if s.creds.PasswordChanged {
		changed = 1
	}
	_, err := s.db.DB().Exec(`
		INSERT INTO auth_account (id, username, password_hash, role, user_id, password_changed, created_at, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			username         = excluded.username,
			password_hash    = excluded.password_hash,
			role             = excluded.role,
			user_id          = excluded.user_id,
			password_changed = excluded.password_changed,
			updated_at       = excluded.updated_at`,
		s.creds.Username,
		s.creds.PasswordHash,
		role,
		s.creds.UserID,
		changed,
		createdAt,
		now,
	)
	return err
}

func (s *AuthService) saveSettingsLocked() error {
	data, err := json.Marshal(s.creds.Settings)
	if err != nil {
		return err
	}
	now := s.now().UTC().Format(time.RFC3339)
	_, err = s.db.DB().Exec(`
		INSERT INTO settings (id, data_json, updated_at)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			data_json  = excluded.data_json,
			updated_at = excluded.updated_at`,
		string(data),
		now,
	)
	return err
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
