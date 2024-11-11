package tools

import (
	"encoding/json"
	"fmt"
	"log"
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
        },
        {
            "type": "function",
            "function": {
                "name": "scrape_web_data",
                "description": "Scrape data from a specified URL using the scraping tool",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "url": {
                            "type": "string",
                            "description": "The full URL of the web page to scrape, e.g. https://r.jina.ai/example"
                        }
                    },
                    "required": [
                        "url"
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
			"scrape_web_data":     NewScrapingTool(),
		},
	}
	log.Printf("Starting call to tool '%s' with arguments: %s", functionName, arguments)
	res := tools.toolsMap[functionName].CallTool(arguments)
	log.Printf("Successfully called tool '%s'. Response: %s", functionName, res)

	return res
}
