package a2a

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// jcsCanonicalize renders a JSON value in the RFC 8785 JSON Canonicalization
// Scheme form: lexicographically sorted object keys, no insignificant
// whitespace, and JCS string and number escaping.
func jcsCanonicalize(value any) ([]byte, error) {
	var out strings.Builder
	if err := jcsWrite(&out, value); err != nil {
		return nil, err
	}
	return []byte(out.String()), nil
}

func jcsWrite(out *strings.Builder, value any) error {
	switch v := value.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if v {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		jcsString(out, v)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("jcs: cannot canonicalize non-finite number")
		}
		out.WriteString(jcsNumber(v))
	case []any:
		out.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := jcsWrite(out, item); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		out.WriteByte('{')
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			return jcsKeyLess(keys[i], keys[j])
		})
		for i, key := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			jcsString(out, key)
			out.WriteByte(':')
			if err := jcsWrite(out, v[key]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("jcs: unsupported value type %T", value)
	}
	return nil
}

// jcsKeyLess orders object keys by UTF-16 code units, as RFC 8785 requires.
func jcsKeyLess(a, b string) bool {
	ra, rb := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ra) && i < len(rb); i++ {
		if ra[i] != rb[i] {
			return ra[i] < rb[i]
		}
	}
	return len(ra) < len(rb)
}

const hexDigits = "0123456789abcdef"

// jcsString writes a JSON string with JCS escaping: only '"', '\' and the
// U+0000..U+001F control characters are escaped, as \uXXXX with lowercase hex.
func jcsString(out *strings.Builder, s string) {
	out.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			out.WriteString(`\"`)
		case r == '\\':
			out.WriteString(`\\`)
		case r == '\b':
			out.WriteString(`\b`)
		case r == '\t':
			out.WriteString(`\t`)
		case r == '\n':
			out.WriteString(`\n`)
		case r == '\f':
			out.WriteString(`\f`)
		case r == '\r':
			out.WriteString(`\r`)
		case r < 0x20:
			out.WriteString(`\u00`)
			out.WriteByte(hexDigits[r>>4])
			out.WriteByte(hexDigits[r&0xf])
		default:
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')
}

// jcsNumber renders a float the way the ECMAScript JSON.stringify algorithm
// does, per RFC 8785 §3.2.2.3. Go's shortest-roundtrip formatting matches
// ES6 for the range of values Agent Cards carry (integers and simple
// decimals); the exponent boundaries are reproduced explicitly.
func jcsNumber(v float64) string {
	if v == 0 {
		return "0"
	}
	abs := math.Abs(v)
	if abs >= 1e21 || abs < 1e-6 {
		s := strconv.FormatFloat(v, 'e', -1, 64)
		// ES6 renders the exponent with an explicit sign: 1e+21, 1.5e-7.
		index := strings.IndexByte(s, 'e')
		if index > 0 {
			mantissa, exponent := s[:index], s[index+1:]
			if len(exponent) > 0 && exponent[0] != '+' && exponent[0] != '-' {
				exponent = "+" + exponent
			}
			s = mantissa + "e" + exponent
		}
		return s
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// jcsFromJSON re-encodes raw JSON as a generic value tree for
// canonicalization. It preserves numbers as float64 and objects as ordered
// map[string]any values.
func jcsFromJSON(data []byte) (any, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return normalizeNumbers(value), nil
}

// normalizeNumbers converts json.Number values to float64 so jcsWrite can
// canonicalize them.
func normalizeNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		f, _ := v.Float64()
		return f
	case []any:
		for i := range v {
			v[i] = normalizeNumbers(v[i])
		}
	case map[string]any:
		for key := range v {
			v[key] = normalizeNumbers(v[key])
		}
	}
	return value
}
