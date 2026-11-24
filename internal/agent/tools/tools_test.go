package tools

import "testing"

func TestSetEnvReplacesExistingKey(t *testing.T) {
	env := []string{"A=1", "MYAAW_AGENT=old", "B=2"}

	got := SetEnv(env, "MYAAW_AGENT", "new")

	count := 0
	for _, entry := range got {
		if entry == "MYAAW_AGENT=new" {
			count++
		}
		if entry == "MYAAW_AGENT=old" {
			t.Errorf("old value was not removed: %v", got)
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one MYAAW_AGENT entry, got %d in %v", count, got)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 entries, got %d: %v", len(got), got)
	}
}

func TestSetEnvAppendsNewKey(t *testing.T) {
	got := SetEnv([]string{"A=1"}, "MYAAW_AGENT_ROOT", "/tmp/agent")

	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(got), got)
	}
	if got[1] != "MYAAW_AGENT_ROOT=/tmp/agent" {
		t.Errorf("unexpected value: %v", got)
	}
}

func TestIsBuiltinEnabledContactAlias(t *testing.T) {
	if isBuiltinEnabled("a2a_contact", map[string]bool{"contact": false}) {
		t.Fatal("a2a_contact should follow legacy contact=false alias")
	}
	if !isBuiltinEnabled("a2a_contact", map[string]bool{"contact": false, "a2a_contact": true}) {
		t.Fatal("explicit a2a_contact=true should take precedence over legacy contact=false")
	}
	if isBuiltinEnabled("contact", map[string]bool{"a2a_contact": false}) {
		t.Fatal("contact lookup should follow a2a_contact=false")
	}
}

func TestIsBuiltinEnabledPythonAlias(t *testing.T) {
	if isBuiltinEnabled("python", map[string]bool{"execute_python": false}) {
		t.Fatal("python should follow legacy execute_python=false alias")
	}
	if !isBuiltinEnabled("python", map[string]bool{"execute_python": false, "python": true}) {
		t.Fatal("explicit python=true should take precedence over legacy execute_python=false")
	}
	if isBuiltinEnabled("execute_python", map[string]bool{"python": false}) {
		t.Fatal("execute_python lookup should follow python=false")
	}
}
