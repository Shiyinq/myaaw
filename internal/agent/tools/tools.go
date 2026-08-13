package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type ToolsContext struct {
	UserID  string
	Channel string
	BaseURL string
}

type ToolsFactory interface {
	CallTool(arguments string, ctx *ToolsContext) string
	ToolDefinition() []byte
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
	registry    = map[string]ToolsFactory{}
	registryMu  sync.RWMutex
	ToolsLogger *log.Logger
)

func init() {
	homeDir, err := os.UserHomeDir()
	if err == nil {
		logDir := filepath.Join(homeDir, ".myaaw", "logs")
		os.MkdirAll(logDir, 0755)
		logPath := filepath.Join(logDir, "tools.log")

		f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			ToolsLogger = log.New(f, "", log.LstdFlags)
			return
		}
	}
	// Fallback to discard if failed
	ToolsLogger = log.New(io.Discard, "", 0)
}

func Register(name string, factory ToolsFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

func ClearRegistryKey(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(registry, name)
}

type ToolsCalling struct {
	toolsMap map[string]ToolsFactory
}

func NewTools(functionName string, arguments string, ctx *ToolsContext) string {
	ToolsLogger.Printf("Starting call to tool '%s' with arguments: %s", functionName, arguments)

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
