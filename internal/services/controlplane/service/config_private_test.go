package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"myaaw/internal/config"
)

func TestSaveAgentConfigKeepsAPIKeysPrivate(t *testing.T) {
	cfg := &config.Config{Providers: map[string]config.ProviderConfig{"openai": {Type: "openai", APIKey: "private-api-key"}}}
	root := setupConfigAgent(t, cfg)
	path := filepath.Join(root, "config", "config.json")
	for _, replace := range []bool{false, true} {
		if !replace {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		} else if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := saveAgentConfig("myfriend", cfg); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
		}
		saved, err := loadAgentConfig("myfriend")
		if err != nil {
			t.Fatal(err)
		}
		if saved.Providers["openai"].APIKey != "private-api-key" {
			t.Fatal("config data lost during private write")
		}
	}
}
