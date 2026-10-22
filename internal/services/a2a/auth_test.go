package a2a

import (
	"testing"

	"myaaw/internal/config"
)

func TestParseCallerEntry(t *testing.T) {
	cases := []struct {
		entry        string
		name         string
		wantRead     bool
		wantDelegate bool
		wantOK       bool
	}{
		{"research:tok", "research", true, true, true},
		{"monitor:tok:read", "monitor", true, false, true},
		{"writer:tok:delegate", "writer", true, true, true},
		{"writer:tok:read|delegate", "writer", true, true, true},
		{"bad", "", false, false, false},
		{":tok", "", false, false, false},
		{"name:", "", false, false, false},
		{"name:tok:bogus", "", false, false, false},
	}

	for _, tc := range cases {
		credential, ok := parseCallerEntry(tc.entry)
		if ok != tc.wantOK {
			t.Fatalf("%q ok = %v, want %v", tc.entry, ok, tc.wantOK)
		}
		if !ok {
			continue
		}
		if credential.caller.Name != tc.name {
			t.Fatalf("%q name = %q, want %q", tc.entry, credential.caller.Name, tc.name)
		}
		if credential.caller.Can(ScopeRead) != tc.wantRead {
			t.Fatalf("%q read = %v, want %v", tc.entry, !tc.wantRead, tc.wantRead)
		}
		if credential.caller.Can(ScopeDelegate) != tc.wantDelegate {
			t.Fatalf("%q delegate = %v, want %v", tc.entry, !tc.wantDelegate, tc.wantDelegate)
		}
	}
}

func TestAuthenticateScopes(t *testing.T) {
	previousCallers := config.A2ACallers
	config.A2ACallers = []string{"monitor:tok-read:read", "research:tok-delegate"}
	t.Cleanup(func() { config.A2ACallers = previousCallers })

	service := NewService(nil, nil)

	monitor, ok := service.Authenticate("tok-read")
	if !ok {
		t.Fatal("read token should authenticate")
	}
	if monitor.Name != "monitor" {
		t.Fatalf("name = %q", monitor.Name)
	}
	if !monitor.Can(ScopeRead) || monitor.Can(ScopeDelegate) {
		t.Fatalf("scopes = %+v", monitor.Scopes)
	}

	research, ok := service.Authenticate("tok-delegate")
	if !ok {
		t.Fatal("delegate token should authenticate")
	}
	if !research.Can(ScopeRead) || !research.Can(ScopeDelegate) {
		t.Fatalf("delegate should imply read: %+v", research.Scopes)
	}

	if _, ok := service.Authenticate("wrong"); ok {
		t.Fatal("unknown token must not authenticate")
	}
}

func TestAuthenticateOpenMode(t *testing.T) {
	previousCallers := config.A2ACallers
	previousToken := config.A2AToken
	previousTokens := config.A2ATokens
	config.A2ACallers = nil
	config.A2AToken = ""
	config.A2ATokens = nil
	t.Cleanup(func() {
		config.A2ACallers = previousCallers
		config.A2AToken = previousToken
		config.A2ATokens = previousTokens
	})

	service := NewService(nil, nil)
	caller, ok := service.Authenticate("")
	if !ok {
		t.Fatal("open mode should accept any caller")
	}
	if !caller.Can(ScopeDelegate) {
		t.Fatalf("open mode scopes = %+v", caller.Scopes)
	}
}
