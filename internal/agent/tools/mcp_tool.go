package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPToolProxy implements ToolsFactory to proxy requests to an MCP Server tool.
type MCPToolProxy struct {
	session mcp.ClientSession
	tool    mcp.Tool
	schema  []byte
}

func cleanSchema(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		cleaned := make(map[string]interface{})
		for k, inner := range val {
			// Strip out unsupported fields in Gemini/OpenAI function calling schemas
			switch k {
			case "$schema", "additionalProperties", "default", "propertyNames",
				"patternProperties", "title", "examples", "$id", "$defs", "$ref":
				continue
			}
			cleaned[k] = cleanSchema(inner)
		}
		return cleaned
	case []interface{}:
		var cleaned []interface{}
		for _, item := range val {
			cleaned = append(cleaned, cleanSchema(item))
		}
		return cleaned
	default:
		return v
	}
}

func NewMCPToolProxy(session mcp.ClientSession, prefixedName string, tool mcp.Tool) *MCPToolProxy {
	// Convert the MCP InputSchema to an OpenAI function schema format
	openAISchema := map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        prefixedName,
			"description": tool.Description,
			"parameters":  cleanSchema(tool.InputSchema),
		},
	}

	schemaBytes, err := json.Marshal(openAISchema)
	if err != nil {
		ToolsLogger.Printf("Warning: Failed to marshal schema for MCP tool %s: %v", tool.Name, err)
	}

	return &MCPToolProxy{
		session: session,
		tool:    tool,
		schema:  schemaBytes,
	}
}

func (t *MCPToolProxy) ToolDefinition() []byte {
	return t.schema
}

func (t *MCPToolProxy) CallTool(arguments string, ctx *ToolsContext) string {
	ToolsLogger.Printf("Executing MCP Tool '%s' with arguments: %s", t.tool.Name, arguments)

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments for MCP tool %s: %v", t.tool.Name, err)
	}

	// Create a context with timeout for the tool execution
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	params := mcp.CallToolParams{
		Name:      t.tool.Name,
		Arguments: args,
	}

	result, err := t.session.CallTool(timeoutCtx, &params)
	if err != nil {
		return fmt.Sprintf("Error calling MCP tool %s: %v", t.tool.Name, err)
	}

	// Format the result
	// MCP results can have text or other types of content
	if len(result.Content) == 0 {
		return "Success (no output returned)"
	}

	var output string
	for _, content := range result.Content {
		switch c := content.(type) {
		case *mcp.TextContent:
			output += c.Text + "\n"
		default:
			output += fmt.Sprintf("[%T content]\n", c)
		}
	}

	if result.IsError {
		return fmt.Sprintf("Error from MCP tool %s:\n%s", t.tool.Name, output)
	}

	return output
}
