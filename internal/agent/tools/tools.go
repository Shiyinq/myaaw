package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"myaaw/internal/agentdir"
)

type ToolsContext struct {
	UserID  string
	Channel string
	BaseURL string
}

// SetEnv returns env with key set to value, replacing any existing entries.
// This avoids duplicate keys when inheriting MYAAW_AGENT from the parent shell.
func SetEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			out = append(out, entry)
		}
	}
	return append(out, prefix+value)
}

type ToolsFactory interface {
	CallTool(arguments string, ctx *ToolsContext) string
	ToolDefinition() []byte
}

// ContextTool is an optional interface for tools that support cancellation
// and deadlines. Tools that do not implement it are executed with
// context.Background().
type ContextTool interface {
	CallToolContext(ctx context.Context, arguments string, tctx *ToolsContext) string
}

// ExecutionMode controls how a tool is scheduled when an assistant message
// contains multiple tool calls.
type ExecutionMode string

const (
	ExecutionSequential ExecutionMode = "sequential"
	ExecutionParallel   ExecutionMode = "parallel"
)

// ExecutionModeAware is an optional interface for tools that can safely run
// concurrently with other tool calls. Tools that do not implement it default
// to sequential execution.
type ExecutionModeAware interface {
	ExecutionMode() ExecutionMode
}

// NamedFactory pairs a registry name with its factory. It is used by the
// agent tool executor to build per-run tool adapters.
type NamedFactory struct {
	Name    string
	Factory ToolsFactory
}

// GetFactories returns the currently registered tool factories, sorted by
// name, excluding the given tool names.
func GetFactories(excluded ...string) []NamedFactory {
	registryMu.RLock()
	defer registryMu.RUnlock()

	excludedSet := make(map[string]struct{}, len(excluded))
	for _, name := range excluded {
		excludedSet[name] = struct{}{}
	}

	factories := make([]NamedFactory, 0, len(registry))
	for name, factory := range registry {
		if _, skip := excludedSet[name]; skip {
			continue
		}
		factories = append(factories, NamedFactory{Name: name, Factory: factory})
	}
	sort.Slice(factories, func(i, j int) bool { return factories[i].Name < factories[j].Name })
	return factories
}

func GetTools(excludedTools ...string) []map[string]interface{} {
	var allTools []map[string]interface{}

	registryMu.RLock()
	defer registryMu.RUnlock()

	for _, factory := range registry {
		var toolDef map[string]interface{}
		err := json.Unmarshal(factory.ToolDefinition(), &toolDef)
		if err != nil {
			log.Printf("Error parsing tool definition: %v", err)
			continue
		}
		allTools = append(allTools, toolDef)
	}

	if len(excludedTools) == 0 {
		return allTools
	}

	var filteredTools []map[string]interface{}
	for _, tool := range allTools {
		if funcMap, ok := tool["function"].(map[string]interface{}); ok {
			name, _ := funcMap["name"].(string)
			excluded := false
			for _, ex := range excludedTools {
				if ex == name {
					excluded = true
					break
				}
			}
			if !excluded {
				filteredTools = append(filteredTools, tool)
			}
		} else {
			filteredTools = append(filteredTools, tool)
		}
	}

	return filteredTools
}

var (
	builtinRegistry = map[string]ToolsFactory{}
	registry        = map[string]ToolsFactory{}
	registryMu      sync.RWMutex
	ToolsLogger     *log.Logger
	toolsLogFile    *os.File
)

func init() {
	ToolsLogger = log.New(io.Discard, "", 0)
	// Package init runs before CLI flags resolve the agent. Open the log file
	// only when the agent actually exists, so meta commands (backup, restore)
	// do not create an empty agent directory on a fresh install.
	if agentdir.AgentInitialized(agentdir.Active()) {
		ConfigureLogging()
	}
}

// ConfigureLogging reopens tools.log for the active agent. It is called after
// CLI flags resolve the agent because package init runs before flag parsing.
func ConfigureLogging() {
	if toolsLogFile != nil {
		_ = toolsLogFile.Close()
		toolsLogFile = nil
	}

	logDir, err := agentdir.ActiveLogsDir()
	if err != nil {
		ToolsLogger = log.New(io.Discard, "", 0)
		return
	}
	_ = os.MkdirAll(logDir, 0755)
	f, err := os.OpenFile(filepath.Join(logDir, "tools.log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		ToolsLogger = log.New(io.Discard, "", 0)
		return
	}
	toolsLogFile = f
	ToolsLogger = log.New(f, "", log.LstdFlags)
}

func Register(name string, factory ToolsFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

func RegisterBuiltin(name string, factory ToolsFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	builtinRegistry[name] = factory
	registry[name] = factory
}

func isBuiltinEnabled(name string, filter map[string]bool) bool {
	if filter == nil {
		return true
	}
	// Check exact match
	if enabled, exists := filter[name]; exists {
		return enabled
	}
	// Support aliases
	if name == "execute_python" {
		if enabled, exists := filter["python"]; exists {
			return enabled
		}
	}
	if name == "python" {
		if enabled, exists := filter["execute_python"]; exists {
			return enabled
		}
	}
	// Default to enabled if not explicitly disabled
	return true
}

func ApplyBuiltinToolsFilter(filter map[string]bool) {
	registryMu.Lock()
	defer registryMu.Unlock()

	for name, factory := range builtinRegistry {
		if isBuiltinEnabled(name, filter) {
			registry[name] = factory
		} else {
			delete(registry, name)
			ToolsLogger.Printf("Built-in tool '%s' disabled by config", name)
		}
	}
}

// GetBuiltinToolNames returns the sorted names of the built-in tools. It is
// independent of the enabled/disabled filter so onboarding can list every tool.
func GetBuiltinToolNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(builtinRegistry))
	for name := range builtinRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ClearRegistryKey(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(registry, name)
}

func GetRegisteredToolNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func LogActiveTools() {
	names := GetRegisteredToolNames()
	ToolsLogger.Printf("Total active tools: %d | Tools: %v", len(names), names)
}

type ToolsCalling struct {
	toolsMap map[string]ToolsFactory
}

func truncateForLog(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...[truncated]"
}

func NewTools(functionName string, arguments string, ctx *ToolsContext) string {
	ToolsLogger.Printf("Starting call to tool '%s' with arguments: %s", functionName, truncateForLog(arguments, 512))

	registryMu.RLock()
	tool, exists := registry[functionName]
	registryMu.RUnlock()

	if !exists {
		errMsg := fmt.Sprintf("Error: tool '%s' not available.", functionName)
		ToolsLogger.Println(errMsg)
		return errMsg
	}

	res := tool.CallTool(arguments, ctx)
	ToolsLogger.Printf("Successfully called tool '%s'", functionName)

	return res
}
