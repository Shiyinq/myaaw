package tools

import (
	"encoding/json"
	"fmt"
	"log"
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
