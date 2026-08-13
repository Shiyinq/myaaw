package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type PluginConfig struct {
	BuiltinTools map[string]bool    `json:"builtin_tools,omitempty"`
	CustomTools  []CustomToolConfig `json:"custom_tools"`
	MCPServers   []MCPServerConfig  `json:"mcp_servers"`
}

type CustomToolConfig struct {
	Name    string                 `json:"name"`
	Command string                 `json:"command"`
	Args    []string               `json:"args,omitempty"`
	Schema  map[string]interface{} `json:"schema,omitempty"`
	Enabled *bool                  `json:"enabled,omitempty"`
}

type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Enabled *bool             `json:"enabled,omitempty"`
}

var (
	pluginWatcher    *fsnotify.Watcher
	externalRegistry = map[string]ToolsFactory{}
	externalMu       sync.RWMutex
)

func LoadExternalTools() {
	// 1. Clear existing external registry
	externalMu.Lock()
	for k := range externalRegistry {
		ClearRegistryKey(k) // Remove from main registry thread-safely
		delete(externalRegistry, k)
	}
	externalMu.Unlock()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		ToolsLogger.Printf("Failed to get home dir for plugins: %v", err)
		return
	}

	toolsDir := filepath.Join(homeDir, ".myaaw", "tools")
	configPath := filepath.Join(toolsDir, "tools.json")

	// Create directories if they don't exist
	os.MkdirAll(toolsDir, 0755)

	// Load JSON Config
	disabledTools := loadConfigTools(configPath)

	// Load Zero-Config Tools
	loadDirectoryTools(toolsDir, disabledTools)

	// Log total active tools and their names as an array
	LogActiveTools()
}

func loadConfigTools(configPath string) map[string]bool {
	disabledTools := make(map[string]bool)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			ToolsLogger.Printf("Warning: Failed to read %s: %v", configPath, err)
		}
		ApplyBuiltinToolsFilter(nil)
		return disabledTools
	}

	var config PluginConfig
	if err := json.Unmarshal(data, &config); err != nil {
		ToolsLogger.Printf("Warning: Failed to parse %s: %v", configPath, err)
		ApplyBuiltinToolsFilter(nil)
		return disabledTools
	}

	// 0. Apply Built-in Tools Filter
	ApplyBuiltinToolsFilter(config.BuiltinTools)

	// 1. Load CLI Tools from config
	for _, ct := range config.CustomTools {
		if ct.Enabled != nil && !*ct.Enabled {
			ToolsLogger.Printf("Custom tool '%s' disabled by config", ct.Name)
			disabledTools[ct.Name] = true
			disabledTools[filepath.Base(ct.Command)] = true
			for _, arg := range ct.Args {
				disabledTools[filepath.Base(arg)] = true
			}
			continue
		}
		schemaBytes, err := json.Marshal(ct.Schema)

		// If schema is not explicitly defined, try running --schema
		if err != nil || len(ct.Schema) == 0 {
			out, err := runSchemaCommand(ct.Command, ct.Args)
			if err != nil {
				ToolsLogger.Printf("[WARN] Skipping CLI tool '%s': %v", ct.Name, err)
				continue
			}
			schemaBytes = out
		}

		// Ensure the schema is valid JSON OpenAI format
		var check map[string]interface{}
		if err := json.Unmarshal(schemaBytes, &check); err != nil {
			ToolsLogger.Printf("[WARN] Skipping CLI tool '%s': schema is not valid JSON", ct.Name)
			continue
		}

		tool := NewCustomTool(ct.Name, ct.Command, ct.Args, schemaBytes)
		externalMu.Lock()
		externalRegistry[ct.Name] = tool
		externalMu.Unlock()

		Register(ct.Name, tool)
	}

	// 2. Load MCP Servers from config
	for _, ms := range config.MCPServers {
		if ms.Enabled != nil && !*ms.Enabled {
			ToolsLogger.Printf("MCP server '%s' disabled by config", ms.Name)
			continue
		}
		startMCPServer(ms)
	}

	return disabledTools
}

func loadDirectoryTools(toolsDir string, disabledTools map[string]bool) {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		ToolsLogger.Printf("Warning: Failed to read tools dir: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "tools.json" {
			continue
		}

		if disabledTools[entry.Name()] {
			continue
		}

		fullPath := filepath.Join(toolsDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Check if executable (user or group or others)
		if info.Mode()&0111 != 0 {
			// Execute with --schema
			out, err := runSchemaCommand(fullPath, nil)
			if err != nil {
				ToolsLogger.Printf("[WARN] Skipping zero-config tool '%s': failed to return schema (%v)", entry.Name(), err)
				continue
			}

			// Validate JSON
			var check map[string]interface{}
			if err := json.Unmarshal(out, &check); err != nil {
				ToolsLogger.Printf("[WARN] Skipping zero-config tool '%s': schema is not valid JSON", entry.Name())
				continue
			}

			// Extract name from schema if possible, else use filename
			name := entry.Name()
			if fn, ok := check["function"].(map[string]interface{}); ok {
				if n, ok := fn["name"].(string); ok && n != "" {
					name = n
				}
			}

			if disabledTools[name] {
				continue
			}

			tool := NewCustomTool(name, fullPath, nil, out)
			externalMu.RLock()
			_, exists := externalRegistry[name]
			externalMu.RUnlock()

			// Avoid overriding if already defined in tools.json
			if !exists {
				externalMu.Lock()
				externalRegistry[name] = tool
				externalMu.Unlock()

				Register(name, tool)
			}
		}
	}
}

func runSchemaCommand(command string, args []string) ([]byte, error) {
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmdArgs := append([]string{}, args...)
	cmdArgs = append(cmdArgs, "--schema")

	resolvedCmd := resolveCommand(command)
	cmd := exec.CommandContext(ctxTimeout, resolvedCmd, cmdArgs...)
	cmd.Env = getUserEnv()
	return cmd.Output()
}

func startMCPServer(ms MCPServerConfig) {
	// Build environment with user's full PATH
	env := getUserEnv()
	for k, v := range ms.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Resolve command to absolute path using user's PATH
	resolvedCmd := resolveCommand(ms.Command)
	cmd := exec.Command(resolvedCmd, ms.Args...)
	cmd.Env = env

	transport := &mcp.CommandTransport{Command: cmd}

	// Create client
	client := mcp.NewClient(
		&mcp.Implementation{Name: "myaaw-agent", Version: "1.0.0"},
		nil,
	)

	// Connect to server
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		ToolsLogger.Printf("[WARN] Failed to connect to MCP server '%s': %v", ms.Name, err)
		return
	}

	// Fetch tools
	toolList, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		ToolsLogger.Printf("[WARN] Failed to fetch tools from MCP server '%s': %v", ms.Name, err)
		return
	}

	for _, t := range toolList.Tools {
		// Use server name + tool name to prevent collisions
		toolName := fmt.Sprintf("%s_%s", ms.Name, t.Name)

		proxy := NewMCPToolProxy(session, toolName, *t)

		externalMu.Lock()
		externalRegistry[toolName] = proxy
		externalMu.Unlock()

		Register(toolName, proxy)
	}
}

func WatchExternalTools() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		ToolsLogger.Printf("Failed to get home dir for tools watcher: %v", err)
		return
	}

	toolsDir := filepath.Join(homeDir, ".myaaw", "tools")
	configPath := filepath.Join(toolsDir, "tools.json")

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		ToolsLogger.Printf("Failed to create tools watcher: %v", err)
		return
	}
	pluginWatcher = watcher

	// Watch config file directory
	configDir := filepath.Dir(configPath)
	watcher.Add(configDir)
	watcher.Add(toolsDir)

	var debounceTimer *time.Timer
	debounceDuration := 500 * time.Millisecond

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				isToolsJSON := filepath.Base(event.Name) == filepath.Base(configPath)
				isToolsDir := strings.HasPrefix(event.Name, toolsDir)

				if isToolsJSON || isToolsDir {
					if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Chmod) || event.Has(fsnotify.Remove) {
						if debounceTimer != nil {
							debounceTimer.Stop()
						}
						debounceTimer = time.AfterFunc(debounceDuration, func() {
							ToolsLogger.Println("Tools configuration changed, reloading...")
							LoadExternalTools()
						})
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				ToolsLogger.Printf("Tools watcher error: %v", err)
			}
		}
	}()
}
