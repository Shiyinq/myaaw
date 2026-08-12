package tools

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

//go:embed tools.json
var embeddedToolsData []byte

type ToolsContext struct {
	UserID  string
	Channel string
	BaseURL string
}

type ToolsFactory interface {
	CallTool(arguments string, ctx *ToolsContext) string
}

func GetTools() []map[string]interface{} {
	// Default to embedded data for standalone binary support
	byteValue := embeddedToolsData

	// Optional: Fallback to local file if available (useful for development)
	workingDir, err := os.Getwd()
	if err == nil {
		filePath := filepath.Join(workingDir, "internal", "agent", "tools", "tools.json")
		if _, err := os.Stat(filePath); err == nil {
			file, err := os.Open(filePath)
			if err == nil {
				defer file.Close()
				if data, err := io.ReadAll(file); err == nil {
					byteValue = data
				}
			}
		}
	}

	var tools []map[string]interface{}
	err = json.Unmarshal(byteValue, &tools)
	if err != nil {
		fmt.Printf("Error unmarshalling tools from tools.json: %v\n", err)
		return nil
	}

	return tools
}

var registry = map[string]ToolsFactory{}

func Register(name string, factory ToolsFactory) {
	registry[name] = factory
}

type ToolsCalling struct {
	toolsMap map[string]ToolsFactory
}

func NewTools(functionName string, arguments string, ctx *ToolsContext) string {
	tools := &ToolsCalling{
		toolsMap: registry,
	}
	log.Printf("Starting call to tool '%s' with arguments: %s", functionName, arguments)

	tool, exists := tools.toolsMap[functionName]
	if !exists {
		errMsg := fmt.Sprintf("Error: tool '%s' not available.", functionName)
		log.Println(errMsg)
		return errMsg
	}

	res := tool.CallTool(arguments, ctx)
	log.Printf("Successfully called tool '%s'. Response: %s", functionName, res)

	return res
}
