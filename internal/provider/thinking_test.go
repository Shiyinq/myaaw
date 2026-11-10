package provider

import (
	"context"
	"testing"

	"myaaw/internal/capabilities"
	"myaaw/internal/thinking"
)

func thinkingBool(v bool) *bool { return &v }

func pinProviderThinkingCatalog(t *testing.T) {
	t.Helper()
	capabilities.SetForTest(capabilities.View{Version: 1, Models: map[string]capabilities.Entry{
		"gpt-6-astra": {
			Provider:     "openai",
			Capabilities: map[string]*bool{"thinking": thinkingBool(true)},
			Thinking: &capabilities.Thinking{
				CanDisable:   thinkingBool(false),
				EffortValues: []string{"low", "medium", "high", "xhigh", "max"},
			},
		},
		"plain-model": {
			Provider:     "openai",
			Capabilities: map[string]*bool{"thinking": thinkingBool(false)},
		},
		"gemini-3.1-pro-preview": {
			Provider:     "gemini",
			Capabilities: map[string]*bool{"thinking": thinkingBool(true)},
			Thinking: &capabilities.Thinking{
				CanDisable:   thinkingBool(false),
				EffortValues: []string{"low", "medium", "high"},
			},
		},
		"gemini-2.5-flash": {
			Provider:     "gemini",
			Capabilities: map[string]*bool{"thinking": thinkingBool(true)},
			Thinking: &capabilities.Thinking{
				CanDisable:   thinkingBool(true),
				EffortValues: []string{"low", "medium", "high"},
			},
		},
		"gemini-3.8-flash": {
			Provider:     "gemini",
			Capabilities: map[string]*bool{"thinking": thinkingBool(true)},
			Thinking: &capabilities.Thinking{
				CanDisable:   thinkingBool(false),
				EffortValues: []string{"low", "medium", "high"},
			},
		},
	}})
	t.Cleanup(capabilities.Reset)
}

func TestApplyThinkingOpenAICompat(t *testing.T) {
	cases := []struct {
		name    string
		variant string
		spec    thinking.Spec
		want    OpenAIRequest
	}{
		{
			name:    "openai level passes verbatim",
			variant: "openai",
			spec:    thinking.Spec{Enabled: true, Level: "xhigh"},
			want:    OpenAIRequest{ReasoningEffort: "xhigh"},
		},
		{
			name:    "openai default omits effort",
			variant: "openai",
			spec:    thinking.Spec{Enabled: true},
			want:    OpenAIRequest{},
		},
		{
			name:    "openai off sends none",
			variant: "openai",
			spec:    thinking.Spec{Enabled: false},
			want:    OpenAIRequest{ReasoningEffort: "none"},
		},
		{
			name:    "deepseek level keeps thinking enabled",
			variant: "deepseek",
			spec:    thinking.Spec{Enabled: true, Level: "max"},
			want:    OpenAIRequest{ReasoningEffort: "max", Thinking: &OpenAIThinking{Type: "enabled"}},
		},
		{
			name:    "deepseek off disables thinking",
			variant: "deepseek",
			spec:    thinking.Spec{Enabled: false},
			want:    OpenAIRequest{Thinking: &OpenAIThinking{Type: "disabled"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req OpenAIRequest
			applyThinking(&req, openAICompatConfig{variant: tc.variant}, "model", tc.spec)
			if req.ReasoningEffort != tc.want.ReasoningEffort {
				t.Fatalf("reasoning_effort = %q, want %q", req.ReasoningEffort, tc.want.ReasoningEffort)
			}
			switch {
			case tc.want.Thinking == nil && req.Thinking != nil:
				t.Fatalf("unexpected thinking field: %+v", req.Thinking)
			case tc.want.Thinking != nil && (req.Thinking == nil || req.Thinking.Type != tc.want.Thinking.Type):
				t.Fatalf("thinking = %+v, want %+v", req.Thinking, tc.want.Thinking)
			}
		})
	}
}

func TestChatGPTReasoningUsesDocumentedEffort(t *testing.T) {
	pinProviderThinkingCatalog(t)

	cases := []struct {
		name    string
		model   string
		level   string
		wantNil bool
		want    chatGPTReasoning
	}{
		{name: "xhigh stays xhigh", model: "gpt-6-astra", level: "xhigh", want: chatGPTReasoning{Summary: "auto", Effort: "xhigh"}},
		{name: "max stays max", model: "gpt-6-astra", level: "max", want: chatGPTReasoning{Summary: "auto", Effort: "max"}},
		{name: "default has no effort", model: "gpt-6-astra", want: chatGPTReasoning{Summary: "auto"}},
		{name: "off not allowed falls back to default", model: "gpt-6-astra", level: "off", want: chatGPTReasoning{Summary: "auto"}},
		{name: "non thinking model omits reasoning", model: "plain-model", level: "high", wantNil: true},
		{name: "legacy model keeps summary", model: "gpt-5.2-codex", want: chatGPTReasoning{Summary: "auto"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := thinking.WithLevel(context.Background(), tc.level)
			got := chatGPTReasoningFor(ctx, tc.model)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("reasoning = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("reasoning is nil")
			}
			if *got != tc.want {
				t.Fatalf("reasoning = %+v, want %+v", *got, tc.want)
			}
		})
	}
}

func TestGeminiBuildRequestThinking(t *testing.T) {
	pinProviderThinkingCatalog(t)

	// Gemini 3: named level on thinkingConfig.
	g := NewGeminiProvider("https://example.invalid", "key", "gemini-3.1-pro-preview").(*GeminiProvider)
	req := g.buildRequest(thinking.WithLevel(context.Background(), "high"), "", nil, nil)
	if req.GenerationConfig == nil || req.GenerationConfig.ThinkingConfig == nil {
		t.Fatalf("thinking config missing: %+v", req.GenerationConfig)
	}
	if !req.GenerationConfig.ThinkingConfig.IncludeThoughts {
		t.Fatal("includeThoughts should be true")
	}
	if got := req.GenerationConfig.ThinkingConfig.ThinkingLevel; got != "high" {
		t.Fatalf("thinking level = %q", got)
	}

	// Default: include thoughts, leave the level to the model.
	req = g.buildRequest(context.Background(), "", nil, nil)
	if req.GenerationConfig == nil || req.GenerationConfig.ThinkingConfig == nil {
		t.Fatalf("default thinking config missing: %+v", req.GenerationConfig)
	}
	if req.GenerationConfig.ThinkingConfig.ThinkingLevel != "" {
		t.Fatalf("default level = %q", req.GenerationConfig.ThinkingConfig.ThinkingLevel)
	}

	// Gemini 3 cannot disable reasoning; off keeps the default.
	req = g.buildRequest(thinking.WithLevel(context.Background(), "off"), "", nil, nil)
	if req.GenerationConfig == nil || req.GenerationConfig.ThinkingConfig == nil || req.GenerationConfig.ThinkingConfig.ThinkingLevel != "" {
		t.Fatalf("off on a non-disableable model = %+v", req.GenerationConfig)
	}

	// Gemini 2.5: budget 0 disables, levels map to documented budgets, and
	// thinkingLevel is never sent.
	flash25 := NewGeminiProvider("https://example.invalid", "key", "gemini-2.5-flash").(*GeminiProvider)
	req = flash25.buildRequest(thinking.WithLevel(context.Background(), "off"), "", nil, nil)
	tc := req.GenerationConfig
	if tc == nil || tc.ThinkingConfig == nil || tc.ThinkingConfig.ThinkingBudget == nil || *tc.ThinkingConfig.ThinkingBudget != 0 {
		t.Fatalf("2.5 off = %+v", tc)
	}
	if tc.ThinkingConfig.ThinkingLevel != "" {
		t.Fatalf("2.5 must not send thinkingLevel: %+v", tc.ThinkingConfig)
	}

	req = flash25.buildRequest(thinking.WithLevel(context.Background(), "high"), "", nil, nil)
	tc = req.GenerationConfig
	if tc == nil || tc.ThinkingConfig == nil || tc.ThinkingConfig.ThinkingLevel != "" {
		t.Fatalf("2.5 level = %+v", tc)
	}
	if tc.ThinkingConfig.ThinkingBudget == nil || *tc.ThinkingConfig.ThinkingBudget != 24576 {
		t.Fatalf("2.5 high budget = %+v", tc.ThinkingConfig.ThinkingBudget)
	}

	// Default keeps the model default: no budget is sent.
	req = flash25.buildRequest(context.Background(), "", nil, nil)
	tc = req.GenerationConfig
	if tc == nil || tc.ThinkingConfig == nil || tc.ThinkingConfig.ThinkingBudget != nil {
		t.Fatalf("2.5 default = %+v", tc)
	}

	// Non-thinking model: no generation config at all.
	plain := NewGeminiProvider("https://example.invalid", "key", "plain-model").(*GeminiProvider)
	req = plain.buildRequest(context.Background(), "", nil, nil)
	if req.GenerationConfig != nil {
		t.Fatalf("non-thinking model gained a generation config: %+v", req.GenerationConfig)
	}
}

func budgetPtr(v int) *int { return &v }

func TestGemini25ThinkingBudget(t *testing.T) {
	cases := []struct {
		model string
		level string
		want  *int
	}{
		{model: "gemini-2.5-flash", level: "", want: nil},
		{model: "gemini-2.5-flash", level: "low", want: budgetPtr(1024)},
		{model: "gemini-2.5-flash", level: "medium", want: budgetPtr(8192)},
		{model: "gemini-2.5-flash", level: "high", want: budgetPtr(24576)},
		{model: "gemini-2.5-flash-lite", level: "minimal", want: budgetPtr(512)},
		{model: "gemini-2.5-pro", level: "high", want: budgetPtr(32768)},
		{model: "gemini-2.5-flash", level: "dynamic", want: budgetPtr(-1)},
		{model: "gemini-2.5-flash", level: "bogus", want: nil},
	}

	for _, tc := range cases {
		t.Run(tc.model+"/"+tc.level, func(t *testing.T) {
			got := gemini25ThinkingBudget(tc.model, tc.level)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("budget = %v, want %v", got, tc.want)
			}
			if got != nil && *got != *tc.want {
				t.Fatalf("budget = %d, want %d", *got, *tc.want)
			}
		})
	}
}
