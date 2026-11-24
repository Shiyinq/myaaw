package python

import (
	"encoding/json"
	"testing"

	"myaaw/internal/agent/tools"
)

func TestPythonToolRegisteredAsPython(t *testing.T) {
	var def struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(NewPythonTool().ToolDefinition(), &def); err != nil {
		t.Fatalf("unmarshal ToolDefinition: %v", err)
	}
	if def.Function.Name != "python" {
		t.Fatalf("ToolDefinition name = %q, want python", def.Function.Name)
	}

	var foundNew, foundOld bool
	for _, name := range tools.GetBuiltinToolNames() {
		if name == "python" {
			foundNew = true
		}
		if name == "execute_python" {
			foundOld = true
		}
	}
	if !foundNew || foundOld {
		t.Fatalf("builtin names = %v, want python without execute_python", tools.GetBuiltinToolNames())
	}
}
