package main

import (
	"os"
	"slices"
	"testing"
)

func withAPIFlags(t *testing.T, port string, terminal bool) {
	t.Helper()
	oldPort, oldHost, oldTerminal := apiPort, apiHost, apiTerminal
	apiPort, apiHost, apiTerminal = port, "127.0.0.1", terminal
	t.Cleanup(func() {
		apiPort, apiHost, apiTerminal = oldPort, oldHost, oldTerminal
	})
}

func TestAPIRunArgsCarriesTerminalFlag(t *testing.T) {
	withAPIFlags(t, "9100", true)

	args := apiRunArgs()
	want := []string{"api", "run", "--port", "9100", "--host", "127.0.0.1", "--terminal"}
	if !slices.Equal(args, want) {
		t.Fatalf("apiRunArgs() = %v, want %v", args, want)
	}
}

func TestAPIRunArgsOmitsTerminalByDefault(t *testing.T) {
	withAPIFlags(t, "9100", false)

	args := apiRunArgs()
	want := []string{"api", "run", "--port", "9100", "--host", "127.0.0.1"}
	if !slices.Equal(args, want) {
		t.Fatalf("apiRunArgs() = %v, want %v", args, want)
	}
}

func TestEnableTerminalFromFlagSetsEnv(t *testing.T) {
	withAPIFlags(t, "9100", true)
	t.Setenv("MYAAW_TERMINAL", "")

	enableTerminalFromFlag()

	if got := os.Getenv("MYAAW_TERMINAL"); got != "1" {
		t.Fatalf("MYAAW_TERMINAL = %q, want %q", got, "1")
	}
}

func TestEnableTerminalFromFlagLeavesEnvAlone(t *testing.T) {
	withAPIFlags(t, "9100", false)
	t.Setenv("MYAAW_TERMINAL", "")

	enableTerminalFromFlag()

	if got := os.Getenv("MYAAW_TERMINAL"); got != "" {
		t.Fatalf("MYAAW_TERMINAL = %q, want empty", got)
	}
}
