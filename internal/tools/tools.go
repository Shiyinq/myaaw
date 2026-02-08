package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"myaaw/internal/tools/bash"

	// "myaaw/internal/tools/calendar"
	// "myaaw/internal/tools/cashflow"
	// "myaaw/internal/tools/converter"
	"myaaw/internal/tools/filesystem"
	// "myaaw/internal/tools/notes"
	"myaaw/internal/tools/python"
	// "myaaw/internal/tools/scraping"
	// "myaaw/internal/tools/tavily"
	// "myaaw/internal/tools/weather"
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

	filePath := filepath.Join(workingDir, "internal", "tools", "tools.json")
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
			// "get_current_weather": weather.NewWeatherTool(),
			// "scrape_web_data":     scraping.NewScrapingTool(),
			// "notes":               notes.NewNotesTool(),
			// "tavily_search":       tavily.NewTavilyTool(),
			// "cash_flow":           cashflow.NewCashFlowTool(),
			// "calendar":            calendar.NewCalendarTool(),
			// "converter":           converter.NewConverterTool(),
			"bash":           bash.NewBashTool(),
			"filesystem":     filesystem.NewFileSystemTool(),
			"execute_python": python.NewPythonTool(),
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
