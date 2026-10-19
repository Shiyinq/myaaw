package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestAuth(t *testing.T) *AuthService {
	t.Helper()
	auth, err := NewAuthServiceAt(filepath.Join(t.TempDir(), "controlplane.json"))
	if err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}
	return auth
}

func TestAuthServiceSeedsDefaultUser(t *testing.T) {
	auth := newTestAuth(t)

	if auth.Username() != DefaultUsername {
		t.Fatalf("username = %q, want %q", auth.Username(), DefaultUsername)
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
	path := filepath.Join(t.TempDir(), "controlplane.json")
	if _, err := NewAuthServiceAt(path); err != nil {
		t.Fatalf("NewAuthServiceAt: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read credentials: %v", err)
	}
	if strings.Contains(string(data), DefaultPassword) {
		t.Fatal("credentials file contains the plaintext password")
	}
	if !strings.Contains(string(data), "password_hash") {
		t.Fatal("credentials file is missing password_hash")
	}
}
