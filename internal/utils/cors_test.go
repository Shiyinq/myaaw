package utils

import "testing"

func TestParseAllowedOrigins(t *testing.T) {
	got := ParseAllowedOrigins(" https://a.example , ,https://b.example,")
	if len(got) != 2 || got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Fatalf("origins = %v", got)
	}

	if got := ParseAllowedOrigins(""); len(got) != 0 {
		t.Fatalf("empty value should yield no origins, got %v", got)
	}
}

func TestIsLoopbackOrigin(t *testing.T) {
	allowed := []string{
		"http://localhost:5050",
		"https://localhost",
		"http://127.0.0.1:9000",
		"http://[::1]:5050",
	}
	for _, origin := range allowed {
		if !IsLoopbackOrigin(origin) {
			t.Fatalf("%q should be loopback", origin)
		}
	}

	blocked := []string{
		"https://evil.example",
		"http://192.168.1.10:5050",
		"https://localhost.evil.example",
		"",
		"not a url",
	}
	for _, origin := range blocked {
		if IsLoopbackOrigin(origin) {
			t.Fatalf("%q should not be loopback", origin)
		}
	}
}
