package main

import (
	"os"
	"slices"
	"testing"
)

func withAPIFlags(t *testing.T, port string, terminal bool) {
	t.Helper()
	oldPort, oldTerminal := apiPort, apiTerminal
	apiPort, apiTerminal = port, terminal
	t.Cleanup(func() {
		apiPort, apiTerminal = oldPort, oldTerminal
	})
}

func TestAPIRunArgsCarriesTerminalFlag(t *testing.T) {
	withAPIFlags(t, "9100", true)

	args := apiRunArgs()
	want := []string{"api", "run", "--port", "9100", "--terminal"}
	if !slices.Equal(args, want) {
		t.Fatalf("apiRunArgs() = %v, want %v", args, want)
	}
}

func TestAPIRunArgsOmitsTerminalByDefault(t *testing.T) {
	withAPIFlags(t, "9100", false)

	args := apiRunArgs()
	want := []string{"api", "run", "--port", "9100"}
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
