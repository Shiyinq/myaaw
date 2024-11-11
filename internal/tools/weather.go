package tools

import (
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

type WeatherTool struct{}

func NewWeatherTool() ToolsFactory {
	return &WeatherTool{}
}

type WeatherArguments struct {
	Location string `json:"location"`
	Unit     string `json:"unit"`
}

func (w *WeatherTool) CallTool(arguments string) string {
	var args WeatherArguments
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	baseURL := fmt.Sprintf("http://wttr.in/%s?format=j1", args.Location)

	client := resty.New()

	var result map[string]interface{}
	resp, err := client.R().SetResult(&result).Get(baseURL)
	if err != nil {
		return fmt.Sprintf("Error making request: %v", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Sprintf("Error response from API: %s", resp.Status())
	}

	currentCondition, ok := result["current_condition"].([]interface{})
	if !ok || len(currentCondition) == 0 {
		return "No current weather data available."
	}

	var temperature string
	if args.Unit == "celsius" {
		temperature, ok = currentCondition[0].(map[string]interface{})["temp_C"].(string)
	} else if args.Unit == "fahrenheit" {
		temperature, ok = currentCondition[0].(map[string]interface{})["temp_F"].(string)
	}

	if !ok {
		return "Temperature data unavailable."
	}

	return fmt.Sprintf("The current temperature in %s is: %s°%s", args.Location, temperature, args.Unit)
}
