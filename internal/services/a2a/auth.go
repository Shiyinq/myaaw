package a2a

import (
	"crypto/subtle"
	"log"
	"strings"

	"myaaw/internal/config"
)

type callerCredential struct {
	token  string
	caller Caller
}

// loadCallers builds the accepted credentials once per service. Plain tokens
// share the "default" identity; A2A_CALLERS adds named callers with scopes.
func (s *Service) loadCallers() {
	credentials := make([]callerCredential, 0)

	if token := strings.TrimSpace(config.A2AToken); token != "" {
		credentials = append(credentials, callerCredential{
			token:  token,
			caller: Caller{Name: "default", Scopes: fullScopes()},
		})
	}
	for _, token := range config.A2ATokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		credentials = append(credentials, callerCredential{
			token:  token,
			caller: Caller{Name: "default", Scopes: fullScopes()},
		})
	}
	for _, entry := range config.A2ACallers {
		credential, ok := parseCallerEntry(entry)
		if !ok {
			log.Printf("a2a: ignoring invalid A2A_CALLERS entry %q", entry)
			continue
		}
		credentials = append(credentials, credential)
	}
	s.callers = credentials
}

func fullScopes() map[string]bool {
	return map[string]bool{ScopeRead: true, ScopeDelegate: true}
}

// parseCallerEntry parses "name:token" or "name:token:scope|scope". When the
// scope list is omitted the caller gets delegate access, which implies read.
func parseCallerEntry(entry string) (callerCredential, bool) {
	parts := strings.SplitN(strings.TrimSpace(entry), ":", 3)
	if len(parts) < 2 {
		return callerCredential{}, false
	}
	name := strings.TrimSpace(parts[0])
	token := strings.TrimSpace(parts[1])
	if name == "" || token == "" {
		return callerCredential{}, false
	}

	scopes := fullScopes()
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		scopes = make(map[string]bool)
		for _, scope := range strings.Split(parts[2], "|") {
			switch strings.ToLower(strings.TrimSpace(scope)) {
			case ScopeRead:
				scopes[ScopeRead] = true
			case ScopeDelegate:
				scopes[ScopeDelegate] = true
				scopes[ScopeRead] = true
			}
		}
		if len(scopes) == 0 {
			return callerCredential{}, false
		}
	}
	return callerCredential{
		token:  token,
		caller: Caller{Name: name, Scopes: scopes},
	}, true
}

// Authenticate resolves a bearer token to a caller. When no credentials are
// configured the endpoint is open and every request is anonymous with full
// access.
func (s *Service) Authenticate(token string) (Caller, bool) {
	if len(s.callers) == 0 {
		return Caller{Name: "anonymous", Scopes: fullScopes()}, true
	}
	token = strings.TrimSpace(token)
	for _, credential := range s.callers {
		if subtle.ConstantTimeCompare([]byte(token), []byte(credential.token)) == 1 {
			caller := credential.caller
			caller.Scopes = cloneScopes(caller.Scopes)
			return caller, true
		}
	}
	return Caller{}, false
}

func cloneScopes(scopes map[string]bool) map[string]bool {
	out := make(map[string]bool, len(scopes))
	for key, value := range scopes {
		out[key] = value
	}
	return out
}
