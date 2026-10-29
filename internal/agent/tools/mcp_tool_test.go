package tools

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestFormatMCPResultTextAndImage(t *testing.T) {
	result := &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: "captured"},
		&mcp.ImageContent{MIMEType: "image/png", Data: []byte("AAAA")},
	}}

	out := formatMCPResult("screenshot", result)
	if !strings.Contains(out.Text, "captured") {
		t.Fatalf("text = %q", out.Text)
	}
	if len(out.Images) != 1 || out.Images[0].MimeType != "image/png" || out.Images[0].Data != "AAAA" {
		t.Fatalf("images = %+v", out.Images)
	}
}

func TestFormatMCPResultErrorAndEmpty(t *testing.T) {
	empty := formatMCPResult("tool", &mcp.CallToolResult{})
	if empty.Text != "Success (no output returned)" {
		t.Fatalf("empty text = %q", empty.Text)
	}

	failed := formatMCPResult("tool", &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: "boom"}},
	})
	if !strings.HasPrefix(failed.Text, "Error from MCP tool tool:") || !strings.Contains(failed.Text, "boom") {
		t.Fatalf("error text = %q", failed.Text)
	}
}
