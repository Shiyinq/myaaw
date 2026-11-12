package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"myaaw/internal/agentdir"
)

const (
	mediaKeyFileName = "media.key"
	// mediaSignatureTTL bounds how long one media URL stays valid. Images already
	// rendered in the browser are unaffected; only new fetches need a fresh
	// signature.
	mediaSignatureTTL = 10 * time.Minute
	// mediaSignatureSkew tolerates a small clock difference between the process
	// that signs and the one that verifies.
	mediaSignatureSkew = 30 * time.Second
)

// MediaSigner issues and verifies per-path signatures for /fs/raw. A signature
// only authorises one file, so a leaked URL cannot be replayed for another.
type MediaSigner struct {
	key []byte
	ttl time.Duration
	now func() time.Time
}

// NewMediaSigner loads (or creates on first start) the signing key.
func NewMediaSigner() (*MediaSigner, error) {
	path, err := agentdir.GlobalConfigPath(mediaKeyFileName)
	if err != nil {
		return nil, err
	}
	key, err := loadOrCreateMediaKey(path)
	if err != nil {
		return nil, err
	}
	ttl := mediaSignatureTTL
	if raw := strings.TrimSpace(os.Getenv("MEDIA_URL_TTL")); raw != "" {
		if parsed, parseErr := time.ParseDuration(raw); parseErr == nil && parsed > 0 {
			ttl = parsed
		}
	}
	return &MediaSigner{key: key, ttl: ttl, now: time.Now}, nil
}

// TTL is how long a signature issued now stays valid.
func (m *MediaSigner) TTL() time.Duration { return m.ttl }

// SignPath returns the signed query suffix and its expiry for one file.
func (m *MediaSigner) SignPath(agent, path string) (string, int64) {
	exp := m.now().Add(m.ttl).Unix()
	sig := m.signature(agent, path, exp)
	return fmt.Sprintf("exp=%d&sig=%s", exp, sig), exp
}

// SignPaths signs a batch so the dashboard can ask for one page's files at once.
func (m *MediaSigner) SignPaths(agent string, paths []string) (int64, map[string]string) {
	suffixes := make(map[string]string, len(paths))
	var exp int64
	for _, path := range paths {
		suffix, at := m.SignPath(agent, path)
		suffixes[path] = suffix
		if at > exp {
			exp = at
		}
	}
	return exp, suffixes
}

// Verify checks a signed query pair. Expired, malformed, or mismatched values
// are rejected.
func (m *MediaSigner) Verify(agent, path, expRaw, sig string) bool {
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil || sig == "" {
		return false
	}
	if exp < m.now().Add(-mediaSignatureSkew).Unix() {
		return false
	}
	expected := m.signature(agent, path, exp)
	return hmac.Equal([]byte(expected), []byte(sig))
}

func (m *MediaSigner) signature(agent, path string, exp int64) string {
	mac := hmac.New(sha256.New, m.key)
	fmt.Fprintf(mac, "%s\x00%s\x00%d", agent, path, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

func loadOrCreateMediaKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil && len(data) >= 32 {
		return data, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			// Another process won the race; use its key.
			return os.ReadFile(path)
		}
		return nil, err
	}
	defer file.Close()
	if _, err := file.Write(key); err != nil {
		return nil, err
	}
	return key, nil
}
