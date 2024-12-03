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

	var temp string
	var tempFeelsLike string
	var weatherDesc string
	currentWeather := currentCondition[0].(map[string]interface{})
	if args.Unit == "celsius" {
		temp = currentWeather["temp_C"].(string)
		tempFeelsLike = currentWeather["FeelsLikeC"].(string)
	} else {
		args.Unit = "fahrenheit"
		temp = currentWeather["temp_F"].(string)
		tempFeelsLike = currentWeather["FeelsLikeF"].(string)
	}

	weather := currentWeather["weatherDesc"].([]interface{})
	for _, item := range weather {
		desc, _ := item.(map[string]interface{})
		weatherDesc += desc["value"].(string) + "\n"
	}

	return fmt.Sprintf(
		"The current weather in %s is %s and temperature is %s°%s (feels like %s°%s).",
		args.Location, weatherDesc, temp, args.Unit, tempFeelsLike, args.Unit,
	)
}
