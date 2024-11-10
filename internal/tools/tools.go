package tools

import (
	"encoding/json"
	"fmt"
)

type ToolsFactory interface {
	CallTool(arguments string) string
}

func GetTools() []map[string]interface{} {
	data := `[
        {
            "type": "function",
            "function": {
                "name": "get_current_weather",
                "description": "Get the current weather in a given location",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "The city and state, e.g. San Francisco, CA"
                        },
                        "unit": {
                            "type": "string",
                            "enum": [
                                "celsius",
                                "fahrenheit"
                            ]
                        }
                    },
                    "required": [
                        "location"
                    ]
                }
            }
        }
    ]`

	var tools []map[string]interface{}
	err := json.Unmarshal([]byte(data), &tools)
	if err != nil {
		fmt.Println("Error unmarshalling data:", err)
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
		},
	}

	return tools.toolsMap[functionName].CallTool(arguments)
}
