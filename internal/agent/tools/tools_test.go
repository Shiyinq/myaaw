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
