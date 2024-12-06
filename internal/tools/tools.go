package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

type ToolsFactory interface {
	CallTool(arguments string) string
}

func GetTools() []map[string]interface{} {
	workingDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Error getting current directory: %v\n", err)
		return nil
	}

	filePath := filepath.Join(workingDir, "internal", "tools", "schemas.json")
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening schemas.json: %v\n", err)
		return nil
	}
	defer file.Close()

	byteValue, err := io.ReadAll(file)
	if err != nil {
		fmt.Printf("Error reading schemas.json: %v\n", err)
		return nil
	}

	var tools []map[string]interface{}
	err = json.Unmarshal(byteValue, &tools)
	if err != nil {
		fmt.Printf("Error unmarshalling schemas from schemas.json: %v\n", err)
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
			"get_current_weather": NewWeatherTool(),
			"scrape_web_data":     NewScrapingTool(),
			"notes":               NewNotesTool(),
		},
	}
	log.Printf("Starting call to tool '%s' with arguments: %s", functionName, arguments)
	res := tools.toolsMap[functionName].CallTool(arguments)
	log.Printf("Successfully called tool '%s'. Response: %s", functionName, res)

	return res
}
