//go:build darwin

package computer

import (
	"strings"
	"testing"
)

func TestAppleScriptKey(t *testing.T) {
	script, err := appleScriptKey("cmd+shift+t")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, `keystroke "t"`) || !strings.Contains(script, "command down") || !strings.Contains(script, "shift down") {
		t.Fatalf("script = %q", script)
	}

	script, err = appleScriptKey("enter")
	if err != nil || !strings.Contains(script, "key code 36") {
		t.Fatalf("script = %q err=%v", script, err)
	}

	if _, err := appleScriptKey("hyper+t"); err == nil {
		t.Fatal("unknown modifier should fail")
	}
	if _, err := appleScriptKey("cmd+f13"); err == nil {
		t.Fatal("modifier on a named key should fail without cliclick")
	}
}

func TestAppleScriptStringEscaping(t *testing.T) {
	got := appleScriptString(`a "quoted" \ path`)
	want := `"a \"quoted\" \\ path"`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
