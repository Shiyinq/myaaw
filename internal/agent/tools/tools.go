package tools

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"myaaw/internal/agent/tools/bash"
	"myaaw/internal/agent/tools/cron"
	"myaaw/internal/agent/tools/filesystem"
	"myaaw/internal/agent/tools/python"
	"os"
	"path/filepath"
)

//go:embed tools.json
var embeddedToolsData []byte

type ToolsFactory interface {
	CallTool(arguments string) string
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

type ToolsCalling struct {
	toolsMap map[string]ToolsFactory
}

func NewTools(functionName string, arguments string) string {
	tools := &ToolsCalling{
		toolsMap: map[string]ToolsFactory{
			"bash":           bash.NewBashTool(),
			"filesystem":     filesystem.NewFileSystemTool(),
			"execute_python": python.NewPythonTool(),
			"cron":           cron.NewCronTool(),
		},
	}
	log.Printf("Starting call to tool '%s' with arguments: %s", functionName, arguments)

	tool, exists := tools.toolsMap[functionName]
	if !exists {
		errMsg := fmt.Sprintf("Error: tool '%s' not available.", functionName)
		log.Println(errMsg)
		return errMsg
	}

	res := tool.CallTool(arguments)
	log.Printf("Successfully called tool '%s'. Response: %s", functionName, res)

	return res
}
