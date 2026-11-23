package service

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func newTestAuth(t *testing.T) *AuthService {
	t.Helper()
	auth, err := NewAuthServiceAt(filepath.Join(t.TempDir(), "controlplane.db"))
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}
	t.Cleanup(func() { _ = auth.Close() })
	return auth
}

func TestAuthServiceSeedsDefaultUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.db")
	auth, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}
	defer auth.Close()

	if auth.Username() != DefaultUsername {
		t.Fatalf("username = %q, want %q", auth.Username(), DefaultUsername)
	}
	if auth.Role() != DefaultRole {
		t.Fatalf("role = %q, want %q", auth.Role(), DefaultRole)
	}
	if auth.UserID() != 0 {
		t.Fatalf("user id = %d, want 0", auth.UserID())
	}
	if !auth.MustChangePassword() {
		t.Fatal("seeded account should require password change")
	}

	var (
		storedUser string
		storedRole string
		storedUID  int
	)
	if err := auth.db.DB().QueryRow(`SELECT username, role, user_id FROM auth_account WHERE id = 1`).
		Scan(&storedUser, &storedRole, &storedUID); err != nil {
		t.Fatalf("query auth_account: %v", err)
	}
	if storedUser != DefaultUsername || storedRole != DefaultRole || storedUID != 0 {
		t.Fatalf("stored row = (%q, %q, %d)", storedUser, storedRole, storedUID)
	}

	token, expiresAt, err := auth.Login(DefaultUsername, DefaultPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token == "" || expiresAt.IsZero() {
		t.Fatalf("token = %q, expiresAt = %v", token, expiresAt)
	}
	if !auth.Validate(token) {
		t.Fatal("token should be valid")
	}
}

func TestAuthServiceLoadsConfiguredUserID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.db")
	first, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}
	if _, err := first.db.DB().Exec(`UPDATE auth_account SET user_id = 42 WHERE id = 1`); err != nil {
		t.Fatalf("update user_id: %v", err)
	}
	_ = first.Close()

	auth, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	defer auth.Close()
	if auth.UserID() != 42 {
		t.Fatalf("user id = %d, want 42", auth.UserID())
	}
	if auth.Role() != DefaultRole {
		t.Fatalf("role = %q, want %q", auth.Role(), DefaultRole)
	}
}

func TestAuthServiceRejectsInvalidCredentials(t *testing.T) {
	auth := newTestAuth(t)

	if _, _, err := auth.Login(DefaultUsername, "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if _, _, err := auth.Login("other", DefaultPassword); err != ErrInvalidCredentials {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if auth.Validate("not-a-token") {
		t.Fatal("unknown token should be invalid")
	}
}

func TestAuthServiceLogoutInvalidatesToken(t *testing.T) {
	auth := newTestAuth(t)

	token, _, err := auth.Login(DefaultUsername, DefaultPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	auth.Logout(token)

	if auth.Validate(token) {
		t.Fatal("token should be invalid after logout")
	}
}

func TestAuthServiceChangePassword(t *testing.T) {
	auth := newTestAuth(t)
	oldToken, _, err := auth.Login(DefaultUsername, DefaultPassword)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := auth.ChangePassword(DefaultPassword, "new-password-123"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if auth.MustChangePassword() {
		t.Fatal("MustChangePassword should be false after ChangePassword")
	}
	if auth.Validate(oldToken) {
		t.Fatal("existing sessions should be invalidated")
	}
	if _, _, err := auth.Login(DefaultUsername, DefaultPassword); err != ErrInvalidCredentials {
		t.Fatalf("old password err = %v, want ErrInvalidCredentials", err)
	}
	if _, _, err := auth.Login(DefaultUsername, "new-password-123"); err != nil {
		t.Fatalf("new password login: %v", err)
	}
}

func TestAuthServiceRejectsWeakPassword(t *testing.T) {
	auth := newTestAuth(t)

	if err := auth.ChangePassword(DefaultPassword, "short"); err != ErrWeakPassword {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
}

func TestAuthServiceChangeUsername(t *testing.T) {
	auth := newTestAuth(t)

	if err := auth.ChangeUsername("wrong", "someone"); err != ErrInvalidCredentials {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if err := auth.ChangeUsername(DefaultPassword, "someone"); err != nil {
		t.Fatalf("ChangeUsername: %v", err)
	}
	if auth.Username() != "someone" {
		t.Fatalf("username = %q", auth.Username())
	}
	if _, _, err := auth.Login("someone", DefaultPassword); err != nil {
		t.Fatalf("login with new username: %v", err)
	}
}

func TestAuthServiceStoresHashNotPlaintext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controlplane.db")
	auth, err := NewAuthServiceAt(path)
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}
	defer auth.Close()

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var hash string
	if err := db.QueryRow(`SELECT password_hash FROM auth_account WHERE id = 1`).Scan(&hash); err != nil {
		t.Fatalf("query password_hash: %v", err)
	}
	if hash == "" || strings.Contains(hash, DefaultPassword) || !strings.HasPrefix(hash, "$2") {
		t.Fatalf("unexpected stored password_hash: %q", hash)
	}
}
