package a2a

import (
	"fmt"
	"os"
	"testing"
)

// TestMain keeps every test in this package away from the developer's real
// ~/.myaaw. The A2A service resolves task directories through the process-wide
// active agent, so a test that forgets to set HOME would otherwise create
// ~/.myaaw/<agent>/jobs/a2a/<task-id> for real.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "myaaw-a2a-test-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp home: %v\n", err)
		os.Exit(1)
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("USERPROFILE", home)

	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
