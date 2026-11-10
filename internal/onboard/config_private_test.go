package onboard

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"myaaw/internal/config"
)

func TestSaveConfigKeepsAPIKeysPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "config.json")
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{"openai": {Type: "openai", APIKey: "private-api-key"}}}
	for _, replace := range []bool{false, true} {
		if replace {
			// Existing installations may still have the old world-readable mode.
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := SaveConfig(path, cfg); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
		}
		saved, err := LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		if saved.Providers["openai"].APIKey != "private-api-key" {
			t.Fatal("config data lost during private write")
		}
	}
	// Non-secret documents retain the generic writer's existing behavior.
	document := filepath.Join(filepath.Dir(path), "model_prices.json")
	if err := WriteFileAtomic(document, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(document)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Fatalf("generic file mode changed to %o", info.Mode().Perm())
	}
}
