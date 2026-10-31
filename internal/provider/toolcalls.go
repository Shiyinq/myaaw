package provider

import (
	"encoding/json"
	"strings"
)

// ToolCallArgumentsString renders tool arguments in the JSON string form that
// OpenAI-compatible providers expect on the wire. A transcript may hold the
// decoded object form, because Gemini answers carry arguments as an object;
// strict servers reject that with a 422 instead of calling the tool.
func ToolCallArgumentsString(args interface{}) string {
	switch value := args.(type) {
	case nil:
		return "{}"
	case string:
		if strings.TrimSpace(value) == "" {
			return "{}"
		}
		return value
	}

	encoded, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
