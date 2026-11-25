package mcpbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	agentruntime "myaaw/internal/agent/runtime"
	"myaaw/internal/agent/tools"
	"myaaw/internal/channel"
	"myaaw/internal/config"
)

// CoreCodingTools are delegated to the external CLI agent's own native tools
// rather than duplicated through the Myaaw MCP bridge.
var CoreCodingTools = []string{
	"filesystem",
	"bash",
	"python",
	"subagent",
	"provider",
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpToolDesc struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Serve runs the JSON-RPC 2.0 MCP stdio server exposing Myaaw ecosystem tools
// (cron, a2a_contact, a2a_auth, a2a_input, extra_messages, computer when
// enabled, and custom tools) to an external CLI runtime.
func Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	bridgeURL := strings.TrimSpace(os.Getenv("MYAAW_BRIDGE_URL"))
	bridgeToken := strings.TrimSpace(os.Getenv("MYAAW_BRIDGE_TOKEN"))

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	enc := json.NewEncoder(out)

	writeResp := func(id json.RawMessage, result any, rpcErr map[string]any) {
		if len(id) == 0 {
			return
		}
		frame := map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
		}
		if rpcErr != nil {
			frame["error"] = rpcErr
		} else {
			frame["result"] = result
		}
		_ = enc.Encode(frame)
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			writeResp(req.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "myaaw-mcp-bridge",
					"version": "1.0.0",
				},
			}, nil)

		case "notifications/initialized":
			// Notification; no response.

		case "ping":
			writeResp(req.ID, map[string]any{}, nil)

		case "tools/list":
			writeResp(req.ID, map[string]any{
				"tools": listBridgeTools(),
			}, nil)

		case "tools/call":
			var params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeResp(req.ID, nil, map[string]any{"code": -32602, "message": "invalid tools/call params"})
				continue
			}
			result, err := callBridgeTool(ctx, params.Name, params.Arguments, bridgeURL, bridgeToken)
			if err != nil {
				writeResp(req.ID, map[string]any{
					"content": []map[string]any{{"type": "text", "text": err.Error()}},
					"isError": true,
				}, nil)
				continue
			}
			writeResp(req.ID, result, nil)

		default:
			if len(req.ID) > 0 {
				writeResp(req.ID, nil, map[string]any{
					"code":    -32601,
					"message": "method not found: " + req.Method,
				})
			}
		}
	}
	return scanner.Err()
}

func listBridgeTools() []mcpToolDesc {
	factories := tools.GetFactories(CoreCodingTools...)
	out := make([]mcpToolDesc, 0, len(factories))
	for _, named := range factories {
		if named.Name == "computer" && !config.ComputerUseEnabled {
			continue
		}
		// Skip MCP proxy tools because user MCP servers are already attached
		// directly to the external agent's own MCP config.
		if strings.HasPrefix(named.Name, "mcp_") {
			continue
		}
		desc, schema := extractToolMeta(named.Factory.ToolDefinition())
		out = append(out, mcpToolDesc{
			Name:        named.Name,
			Description: desc,
			InputSchema: schema,
		})
	}
	return out
}

func extractToolMeta(raw []byte) (string, json.RawMessage) {
	var def struct {
		Function struct {
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &def); err != nil {
		return "", json.RawMessage(`{"type":"object","properties":{}}`)
	}
	schema := def.Function.Parameters
	if len(schema) == 0 {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	return def.Function.Description, schema
}

func callBridgeTool(ctx context.Context, name string, args json.RawMessage, bridgeURL, bridgeToken string) (map[string]any, error) {
	for _, blocked := range CoreCodingTools {
		if name == blocked {
			return nil, fmt.Errorf("tool %q is delegated to the external runtime", name)
		}
	}
	if name == "computer" && !config.ComputerUseEnabled {
		return nil, fmt.Errorf("tool %q is disabled", name)
	}

	var target tools.ToolsFactory
	for _, named := range tools.GetFactories(CoreCodingTools...) {
		if named.Name == name {
			target = named.Factory
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("unknown myaaw tool %q", name)
	}

	tctx := agentruntime.FetchTurnContext(ctx, bridgeURL, bridgeToken)
	if tctx == nil {
		tctx = &tools.ToolsContext{BaseURL: defaultToolsBaseURL()}
	}
	if tctx.BaseURL == "" {
		tctx.BaseURL = defaultToolsBaseURL()
	}

	argStr := strings.TrimSpace(string(args))
	if argStr == "" || argStr == "null" {
		argStr = "{}"
	}

	if structured, ok := target.(tools.StructuredTool); ok {
		sr := structured.CallToolStructured(ctx, argStr, tctx)
		if len(sr.ExtraMessages) > 0 || len(sr.Attachments) > 0 {
			atts := make([]channel.Attachment, 0, len(sr.Attachments))
			for _, a := range sr.Attachments {
				atts = append(atts, channel.Attachment{
					FileName: a.FileName,
					MimeType: a.MimeType,
					Data:     a.Data,
					Path:     a.Path,
				})
			}
			agentruntime.PushTurnSideEffects(ctx, bridgeURL, bridgeToken, agentruntime.BridgeSideEffectPayload{
				ExtraMessages: sr.ExtraMessages,
				Attachments:   atts,
			})
		}
		content := make([]map[string]any, 0, len(sr.Images)+1)
		if sr.Text != "" || len(sr.Images) == 0 {
			content = append(content, map[string]any{"type": "text", "text": sr.Text})
		}
		for _, img := range sr.Images {
			if strings.TrimSpace(img.Data) == "" {
				continue
			}
			mime := img.MimeType
			if mime == "" {
				mime = "image/png"
			}
			content = append(content, map[string]any{
				"type":     "image",
				"data":     img.Data,
				"mimeType": mime,
			})
		}
		return map[string]any{
			"content": content,
			"isError": looksLikeError(sr.Text),
		}, nil
	}

	var output string
	if ctxTool, ok := target.(tools.ContextTool); ok {
		output = ctxTool.CallToolContext(ctx, argStr, tctx)
	} else {
		output = target.CallTool(argStr, tctx)
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": output}},
		"isError": looksLikeError(output),
	}, nil
}

func defaultToolsBaseURL() string {
	if config.MYAAWBaseURL != "" {
		return config.MYAAWBaseURL
	}
	port := strings.TrimPrefix(config.PORT, ":")
	if port == "" {
		port = "8080"
	}
	return "http://localhost:" + port
}

func looksLikeError(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(lower, "error") || strings.HasPrefix(lower, "failed")
}
