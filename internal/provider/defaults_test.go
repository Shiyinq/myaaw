package provider

import "testing"

func TestDefaultBaseURL(t *testing.T) {
	cases := map[string]string{
		"openai": "https://api.openai.com",
		"gemini": "https://generativelanguage.googleapis.com",
		"groq":   "https://api.groq.com/openai",
		"ollama": "http://localhost:11434",
		"other":  "",
	}
	for name, want := range cases {
		if got := DefaultBaseURL(name); got != want {
			t.Fatalf("DefaultBaseURL(%q) = %q, want %q", name, got, want)
		}
	}
}
