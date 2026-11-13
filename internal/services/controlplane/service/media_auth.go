package service

import "fmt"

// SignMedia signs one page of media paths for the dashboard.
func (s *AuthService) SignMedia(agent string, paths []string) (int64, map[string]string, error) {
	if s.media == nil {
		return 0, nil, fmt.Errorf("media signing is unavailable")
	}
	exp, suffixes := s.media.SignPaths(agent, paths)
	return exp, suffixes, nil
}

// ValidateMediaSignature authorises a signed /fs/raw request without a bearer
// token. The signature covers the agent, the path, and its expiry.
func (s *AuthService) ValidateMediaSignature(agent, path, exp, sig string) bool {
	if s.media == nil {
		return false
	}
	return s.media.Verify(agent, path, exp, sig)
}

// SignProjectPreview issues a short-lived preview token for one project root.
func (s *AuthService) SignProjectPreview(agent, path string) (string, int64, error) {
	if s.media == nil {
		return "", 0, fmt.Errorf("media signing is unavailable")
	}
	token, exp := s.media.SignProjectToken(agent, path)
	return token, exp, nil
}

// ValidateProjectPreview authorises a preview request without a bearer token.
func (s *AuthService) ValidateProjectPreview(agent, token string) (string, bool) {
	if s.media == nil {
		return "", false
	}
	return s.media.VerifyProjectToken(agent, token)
}

// newMediaSignerOrNil keeps the server running when the key cannot be created;
// signed URLs stay disabled and the WebUI falls back to the session token.
func newMediaSignerOrNil() *MediaSigner {
	signer, err := NewMediaSigner()
	if err != nil {
		return nil
	}
	return signer
}
