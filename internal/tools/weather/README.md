# Weather Tool

A tool for retrieving current weather information for any location using the wttr.in API.

## Overview

The Weather Tool provides real-time weather data including temperature, weather description, and "feels like" temperature for any specified location. It supports both Celsius and Fahrenheit temperature units.

## Features

- **Current Weather Data**: Get real-time weather information
- **Temperature Units**: Support for both Celsius and Fahrenheit
- **Weather Description**: Detailed weather conditions
- **Feels Like Temperature**: Perceived temperature based on humidity and wind
- **Global Coverage**: Works with any city/location worldwide

## API Endpoint

Uses the `wttr.in` API service which provides weather data in JSON format.

## Usage

### Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `location` | string | Yes | The city and state/location | "San Francisco, CA" |
| `unit` | string | No | Temperature unit (celsius/fahrenheit) | "celsius" |

### Example Usage

```json
{
  "location": "Jakarta, Indonesia",
  "unit": "celsius"
}
```

### Response Format

The tool returns a human-readable string with weather information:

```
The current weather in Jakarta, Indonesia is Partly cloudy and temperature is 28°celsius (feels like 30°celsius).
```

## Implementation Details

### Dependencies

- `github.com/go-resty/resty/v2` - HTTP client for API requests

### Key Functions

- `NewWeatherTool()` - Creates a new weather tool instance
- `CallTool(arguments string)` - Main function that processes weather requests

### Data Processing

1. Parses JSON arguments for location and unit
2. Constructs API URL with location parameter
3. Makes HTTP GET request to wttr.in API
4. Extracts relevant weather data from JSON response
5. Formats response based on specified temperature unit
6. Returns formatted weather information

## Error Handling

- Invalid argument parsing
- Network request failures
- API response errors
- Missing weather data

## Temperature Unit Support

- **Celsius**: Uses `temp_C` and `FeelsLikeC` from API
- **Fahrenheit**: Uses `temp_F` and `FeelsLikeF` from API
- **Default**: Falls back to Celsius if unit is not specified

## Security Considerations

- No API key required (uses public wttr.in service)
- Input validation for location parameter
- Error handling for malformed responses

## Limitations

- Dependent on wttr.in service availability
- Rate limiting may apply
- Weather data accuracy depends on the underlying weather service
- No historical weather data support
