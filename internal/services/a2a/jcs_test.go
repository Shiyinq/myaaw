package a2a

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestJCSCanonicalizationRFC8785Vector uses the official RFC 8785 §3.2.2 test
// vector, which exercises key sorting, string escaping, and number
// serialization together.
func TestJCSCanonicalizationRFC8785Vector(t *testing.T) {
	input := `{"numbers": [333333333.33333329, 1E30, 4.50, 2e-3, 0.000000000000000000000000001], "string": "\u20ac$\u000F\u000aA'\u0042\u0022\u005c\\\"\/", "literals": [null, true, false]}`
	expected := `{"literals":[null,true,false],"numbers":[333333333.3333333,1e+30,4.5,0.002,1e-27],"string":"€$\u000f\nA'B\"\\\\\"/"}`

	generic, err := jcsFromJSON([]byte(input))
	if err != nil {
		t.Fatalf("jcsFromJSON: %v", err)
	}
	got, err := jcsCanonicalize(generic)
	if err != nil {
		t.Fatalf("jcsCanonicalize: %v", err)
	}
	if string(got) != expected {
		t.Fatalf("canonical form:\n got: %s\nwant: %s", got, expected)
	}
}

func TestJCSCanonicalizeSortsKeys(t *testing.T) {
	input := `{"b":1,"a":2,"c":{"d":3,"b":4}}`
	expected := `{"a":2,"b":1,"c":{"b":4,"d":3}}`

	generic, err := jcsFromJSON([]byte(input))
	if err != nil {
		t.Fatalf("jcsFromJSON: %v", err)
	}
	got, err := jcsCanonicalize(generic)
	if err != nil {
		t.Fatalf("jcsCanonicalize: %v", err)
	}
	if string(got) != expected {
		t.Fatalf("canonical form:\n got: %s\nwant: %s", got, expected)
	}
}

func TestJCSCanonicalizeUnicodeKeys(t *testing.T) {
	input := `{"é":1,"e":2,"😀":3}`
	generic, err := jcsFromJSON([]byte(input))
	if err != nil {
		t.Fatalf("jcsFromJSON: %v", err)
	}
	got, err := jcsCanonicalize(generic)
	if err != nil {
		t.Fatalf("jcsCanonicalize: %v", err)
	}
	// UTF-16 code unit order: "e" (0x65) < "é" (0xE9) < "😀" (surrogates).
	if string(got) != `{"e":2,"é":1,"😀":3}` {
		t.Fatalf("canonical form = %s", got)
	}
}

func TestJCSCanonicalizeEscapesControlChars(t *testing.T) {
	input := "\"\\u0000\\u0008\\u001f\\u2028\""
	// U+2028 is not a control character, so JCS must NOT escape it.
	expected := "\"\\u0000\\b\\u001f\u2028\""
	generic, err := jcsFromJSON([]byte(input))
	if err != nil {
		t.Fatalf("jcsFromJSON: %v", err)
	}
	got, err := jcsCanonicalize(generic)
	if err != nil {
		t.Fatalf("jcsCanonicalize: %v", err)
	}
	if string(got) != expected {
		t.Fatalf("canonical form:\n got: %s\nwant: %s", got, expected)
	}
}

func TestCanonicalCardDropsSignaturesAndWhitespace(t *testing.T) {
	card := BuildCard("echo", "http://localhost:8080", nil, false)
	signed, err := SignCard(card, mustSigningKey(t))
	if err != nil {
		t.Fatalf("SignCard: %v", err)
	}
	unsigned := signed
	unsigned.Signatures = nil

	payload, err := canonicalCard(signed)
	if err != nil {
		t.Fatalf("canonicalCard: %v", err)
	}
	if strings.Contains(string(payload), "signature") || strings.Contains(string(payload), "protected") {
		t.Fatalf("signed payload must exclude signatures: %s", payload)
	}

	var tree map[string]any
	if err := json.Unmarshal(payload, &tree); err != nil {
		t.Fatalf("canonical payload is not valid JSON: %v", err)
	}
	if tree["name"] != unsigned.Name {
		t.Fatalf("payload tree = %+v", tree)
	}
	if _, ok := tree["supportedInterfaces"]; !ok {
		t.Fatalf("required supportedInterfaces missing: %s", payload)
	}
}
