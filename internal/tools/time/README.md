# Time Tool

A tool for retrieving current time information for any timezone using Go's time package.

## Overview

The Time Tool provides current time information including date, time, and weekday for any specified timezone. It uses Go's built-in time package to handle timezone conversions and formatting.

## Features

- **Current Time**: Get current time in any timezone
- **Timezone Support**: Full IANA timezone database support
- **Structured Output**: JSON format with detailed time components
- **Local Timezone**: Default to system local timezone
- **Error Handling**: Comprehensive timezone validation

## Usage

### Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `timezone` | string | No | IANA timezone identifier | "America/New_York" |

### Example Usage

#### Get Local Time

```json
{
  "timezone": ""
}
```

#### Get Specific Timezone

```json
{
  "timezone": "Asia/Jakarta"
}
```

#### Get UTC Time

```json
{
  "timezone": "UTC"
}
```

### Response Format

The tool returns a JSON object with detailed time information:

```json
{
  "current_time": "2024-01-01T10:30:45.123456789-05:00",
  "timezone": "America/New_York",
  "year": 2024,
  "month": 1,
  "day": 1,
  "hour": 10,
  "minute": 30,
  "second": 45,
  "weekday": "Monday"
}
```

## Implementation Details

### Dependencies

- Standard Go `time` package - No external dependencies

### Key Functions

- `NewTimeTool()` - Creates new time tool instance
- `CallTool(arguments string)` - Main function that processes time requests

### Data Processing

1. **Input Parsing**: Parses JSON arguments for timezone
2. **Timezone Loading**: Loads specified timezone or defaults to local
3. **Time Calculation**: Gets current time in specified timezone
4. **Component Extraction**: Extracts individual time components
5. **JSON Marshaling**: Formats response as JSON

## Timezone Support

### IANA Timezone Database

The tool supports all timezones in the IANA timezone database, including:

- **Continental**: America/New_York, Europe/London, Asia/Tokyo
- **Regional**: UTC, GMT, EST, PST
- **City-based**: Asia/Jakarta, Europe/Paris, America/Los_Angeles

### Common Timezones

| Timezone | Description |
|----------|-------------|
| `UTC` | Coordinated Universal Time |
| `America/New_York` | Eastern Time |
| `America/Los_Angeles` | Pacific Time |
| `Europe/London` | British Time |
| `Asia/Tokyo` | Japan Standard Time |
| `Asia/Jakarta` | Western Indonesian Time |

## Response Components

### Time Components

- **current_time**: Full ISO 8601 timestamp with timezone offset
- **timezone**: IANA timezone identifier
- **year**: Four-digit year
- **month**: Month number (1-12)
- **day**: Day of month (1-31)
- **hour**: Hour in 24-hour format (0-23)
- **minute**: Minute (0-59)
- **second**: Second (0-59)
- **weekday**: Full weekday name

### Time Format

- **ISO 8601**: Standard international time format
- **Timezone Offset**: Includes timezone offset information
- **Nanosecond Precision**: Full time precision

## Error Handling

### Invalid Timezone

```json
{
  "error": "Invalid timezone: Invalid/Timezone"
}
```

### Invalid Arguments

```json
{
  "error": "Invalid arguments format: unexpected end of JSON input"
}
```

### Marshaling Errors

```json
{
  "error": "Failed to marshal response: ..."
}
```

## Use Cases

- **Scheduling**: Get current time for scheduling operations
- **Logging**: Timestamp generation for logs
- **Timezone Conversion**: Convert between different timezones
- **Date Calculations**: Get current date components
- **Time-sensitive Operations**: Operations requiring current time

## Best Practices

- Use IANA timezone identifiers for maximum compatibility
- Handle timezone errors gracefully
- Consider daylight saving time transitions
- Use UTC for system operations when possible
- Validate timezone strings before use

## Limitations

- No historical time support
- No timezone conversion between different times
- No custom time formatting options
- No time arithmetic operations
- Dependent on system time accuracy

## Security Considerations

- No external API calls
- No sensitive data exposure
- Input validation for timezone strings
- Error message sanitization

## Performance

- Fast execution (uses Go's built-in time package)
- No network requests
- Minimal memory usage
- Efficient timezone database access

## Examples

### Get Current Time in Different Timezones

```json
// New York
{"timezone": "America/New_York"}

// London
{"timezone": "Europe/London"}

// Tokyo
{"timezone": "Asia/Tokyo"}

// Jakarta
{"timezone": "Asia/Jakarta"}
```

### System Local Time

```json
{}
```

This will return the current time in the system's local timezone.
