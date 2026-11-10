package thinking

import (
	"context"
	"testing"

	"myaaw/internal/capabilities"
)

func boolPtr(v bool) *bool { return &v }

func pinCatalog(t *testing.T) {
	t.Helper()
	capabilities.SetForTest(capabilities.View{Version: 1, Models: map[string]capabilities.Entry{
		"reasoner": {
			Capabilities: map[string]*bool{"thinking": boolPtr(true)},
			Thinking: &capabilities.Thinking{
				CanDisable:   boolPtr(true),
				EffortValues: []string{"none", "low", "high", "xhigh", "max"},
			},
		},
		"no-off": {
			Capabilities: map[string]*bool{"thinking": boolPtr(true)},
			Thinking: &capabilities.Thinking{
				CanDisable:   boolPtr(false),
				EffortValues: []string{"low", "medium", "high"},
			},
		},
		"plain": {Capabilities: map[string]*bool{"thinking": boolPtr(false)}},
	}})
	t.Cleanup(capabilities.Reset)
}

func TestResolve(t *testing.T) {
	pinCatalog(t)

	cases := []struct {
		name     string
		model    string
		level    string
		legacy   bool
		wantOK   bool
		wantSpec Spec
	}{
		{name: "non thinking model", model: "plain", level: "high", wantOK: false},
		{name: "unknown without legacy", model: "unknown", level: "high", wantOK: false},
		{name: "unknown with legacy", model: "unknown", level: "", legacy: true, wantOK: true, wantSpec: Spec{Enabled: true}},
		{name: "default", model: "reasoner", level: "", wantOK: true, wantSpec: Spec{Enabled: true}},
		{name: "documented level", model: "reasoner", level: "xhigh", wantOK: true, wantSpec: Spec{Enabled: true, Level: "xhigh"}},
		{name: "max stays max", model: "reasoner", level: "max", wantOK: true, wantSpec: Spec{Enabled: true, Level: "max"}},
		{name: "case insensitive", model: "reasoner", level: "HIGH", wantOK: true, wantSpec: Spec{Enabled: true, Level: "high"}},
		{name: "off when disable allowed", model: "reasoner", level: "off", wantOK: true, wantSpec: Spec{Enabled: false}},
		{name: "none when documented", model: "reasoner", level: "none", wantOK: true, wantSpec: Spec{Enabled: false}},
		{name: "off when disable forbidden", model: "no-off", level: "off", wantOK: true, wantSpec: Spec{Enabled: true}},
		{name: "unknown level degrades to default", model: "no-off", level: "bogus", wantOK: true, wantSpec: Spec{Enabled: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := Resolve(tc.model, tc.level, tc.legacy)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if spec != tc.wantSpec {
				t.Fatalf("spec = %+v, want %+v", spec, tc.wantSpec)
			}
		})
	}
}

func TestLevelContextRoundTrip(t *testing.T) {
	ctx := WithLevel(context.Background(), " high ")
	if got := Level(ctx); got != "high" {
		t.Fatalf("level = %q", got)
	}
	if got := Level(context.Background()); got != "" {
		t.Fatalf("empty context = %q", got)
	}
	if got := Level(nil); got != "" {
		t.Fatalf("nil context = %q", got)
	}
}
