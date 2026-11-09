package main

import (
	"os"
	"slices"
	"testing"
)

func withAPIFlags(t *testing.T, port string, terminal bool) {
	t.Helper()
	oldPort, oldHost, oldTerminal, oldOrigins := apiPort, apiHost, apiTerminal, apiAllowedOrigins
	apiPort, apiHost, apiTerminal, apiAllowedOrigins = port, "127.0.0.1", terminal, ""
	t.Cleanup(func() {
		apiPort, apiHost, apiTerminal, apiAllowedOrigins = oldPort, oldHost, oldTerminal, oldOrigins
	})
	t.Setenv("ALLOWED_ORIGINS", "")
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

func TestAPIRunArgsCarriesAllowedOrigins(t *testing.T) {
	withAPIFlags(t, "9100", false)
	apiAllowedOrigins = " https://a.example , https://b.example "

	args := apiRunArgs()
	want := []string{
		"api", "run", "--port", "9100", "--host", "127.0.0.1",
		"--allowed-origins", "https://a.example,https://b.example",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("apiRunArgs() = %v, want %v", args, want)
	}
}

func TestAPIRunArgsForwardsAllowedOriginsFromEnv(t *testing.T) {
	withAPIFlags(t, "9100", false)
	t.Setenv("ALLOWED_ORIGINS", "https://env.example")

	args := apiRunArgs()
	want := []string{
		"api", "run", "--port", "9100", "--host", "127.0.0.1",
		"--allowed-origins", "https://env.example",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("apiRunArgs() = %v, want %v", args, want)
	}
}

func TestApplyAPIFlagsSetsTerminal(t *testing.T) {
	withAPIFlags(t, "9100", true)
	t.Setenv("MYAAW_TERMINAL", "")

	applyAPIFlags()

	if got := os.Getenv("MYAAW_TERMINAL"); got != "1" {
		t.Fatalf("MYAAW_TERMINAL = %q, want %q", got, "1")
	}
}

func TestApplyAPIFlagsLeavesTerminalAlone(t *testing.T) {
	withAPIFlags(t, "9100", false)
	t.Setenv("MYAAW_TERMINAL", "")

	applyAPIFlags()

	if got := os.Getenv("MYAAW_TERMINAL"); got != "" {
		t.Fatalf("MYAAW_TERMINAL = %q, want empty", got)
	}
}

func TestApplyAPIFlagsSetsAllowedOrigins(t *testing.T) {
	withAPIFlags(t, "9100", false)
	t.Setenv("ALLOWED_ORIGINS", "")
	apiAllowedOrigins = "https://a.example,https://b.example"

	applyAPIFlags()

	if got := os.Getenv("ALLOWED_ORIGINS"); got != "https://a.example,https://b.example" {
		t.Fatalf("ALLOWED_ORIGINS = %q", got)
	}
}
