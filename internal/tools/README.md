# TEO Tools Documentation

A comprehensive collection of tools for the TEO application, providing various functionalities for data management, web services, file operations, and more.

## Overview

The TEO Tools package contains a set of specialized tools that can be used within the TEO application. Each tool is designed to handle specific tasks and provides a standardized interface for integration.

## Available Tools

### 1. [Weather Tool](./weather/README.md)
**Function**: `get_current_weather`
- Retrieves current weather information for any location
- Supports Celsius and Fahrenheit temperature units
- Uses wttr.in API for real-time weather data

### 2. [Web Scraping Tool](./scraping/README.md)
**Function**: `scrape_web_data`
- Extracts content from web pages using Jina AI Reader API
- Simple URL-based interface
- Returns raw extracted content

### 3. [Notes Management Tool](./notes/README.md)
**Function**: `notes`
- Complete note-taking solution with CRUD operations
- Search functionality and date-based filtering
- JSON storage with metadata tracking

### 4. [File System Tool](./filesystem/README.md)
**Function**: `filesystem`
- Comprehensive file and directory management
- Security-restricted to allowed directories
- Multiple operations: read, write, edit, move, delete, search

### 5. [Tavily Search Tool](./tavily/README.md)
**Function**: `tavily_search`
- AI-powered web search and content extraction
- Configurable search depth and topic filtering
- Requires TAVILY_API_KEY environment variable

### 6. [Time Tool](./time/README.md)
**Function**: `get_time`
- Retrieves current time information for any timezone
- Full IANA timezone database support
- Structured JSON output with detailed time components

### 7. [Cash Flow Tool](./cashflow/README.md)
**Function**: `cash_flow`
- Personal/business cash flow management
- Transaction tracking with categorization
- Financial analytics and multi-currency support

### 8. [Unit Converter Tool](./converter/README.md)
**Function**: `converter`
- Converts values between different measurement units
- Supports temperature, distance, mass, volume, time, speed
- Precise calculations with comprehensive unit support

### 9. [Calendar Tool](./calendar/README.md)
**Function**: `calendar`
- Event scheduling and management
- Date range, title, and tag-based searches
- Full CRUD operations for calendar events

### 10. [Python Execution Tool](./python/README.md)
**Function**: `execute_python`
- Dynamic Python code execution
- Package installation support
- Temporary environment with input/output handling

## Tool Integration

### Factory Pattern
All tools implement the `ToolsFactory` interface:

```go
type ToolsFactory interface {
    CallTool(arguments string) string
}
```

### Tool Registration
Tools are registered in the `toolsMap` within `NewTools()`:

```go
toolsMap: map[string]ToolsFactory{
    "get_current_weather": weather.NewWeatherTool(),
    "scrape_web_data":     scraping.NewScrapingTool(),
    "notes":               notes.NewNotesTool(),
    "filesystem":          filesystem.NewFileSystemTool(),
    "tavily_search":       tavily.NewTavilyTool(),
    "get_time":            time.NewTimeTool(),
    "cash_flow":           cashflow.NewCashFlowTool(),
    "calendar":            calendar.NewCalendarTool(),
    "converter":           converter.NewConverterTool(),
    "execute_python":      python.NewPythonTool(),
}
```

## Configuration

### Environment Variables
Some tools require environment variables:

- **Tavily Tool**: `TAVILY_API_KEY` - API key for Tavily search service

### Data Storage
Tools with persistent data store files in the `data/` directory:

- **Notes**: `data/notes/`
- **Cash Flow**: `data/cashflow/cashflow.json`
- **Calendar**: `data/calendar/calendar.json`

### Security
- **File System Tool**: Restricted to `~/teo_home` directory
- **Python Tool**: Temporary execution environment
- **All Tools**: Input validation and error handling

## Usage Examples

### Basic Tool Call
```go
result := NewTools("get_current_weather", `{"location": "Jakarta", "unit": "celsius"}`)
```

### Tool with Complex Parameters
```go
result := NewTools("notes", `{
    "action": "POST",
    "title": "Meeting Notes",
    "content": "Discussion about project timeline"
}`)
```

## Error Handling

All tools provide comprehensive error handling:

- **Input Validation**: Validates required parameters
- **JSON Parsing**: Handles malformed JSON input
- **External API Errors**: Manages network and API failures
- **File System Errors**: Handles file operations gracefully
- **Security Violations**: Prevents unauthorized access

## Performance Considerations

- **Local Tools**: Fast execution with minimal overhead
- **API Tools**: Subject to external service availability and rate limits
- **File Operations**: Efficient JSON file handling
- **Python Execution**: Temporary environment with automatic cleanup

## Security Features

- **Input Sanitization**: All user inputs are validated
- **Path Restrictions**: File system access is limited to safe directories
- **Temporary Execution**: Python code runs in isolated environments
- **Error Message Sanitization**: Prevents information leakage
- **No System Access**: Tools cannot access system-level resources

## Best Practices

### Tool Selection
- Choose the most appropriate tool for your task
- Consider performance implications for API-based tools
- Use local tools for sensitive data operations

### Error Handling
- Always handle tool execution errors
- Validate tool responses before processing
- Implement fallback mechanisms for critical operations

### Data Management
- Regular backups of tool data files
- Monitor storage usage for persistent tools
- Clean up temporary data when appropriate

### Security
- Validate all tool inputs
- Use appropriate permissions for file operations
- Monitor tool usage for security concerns

## Development

### Adding New Tools
1. Create a new directory in `internal/tools/`
2. Implement the `ToolsFactory` interface
3. Add tool registration in `tools.go`
4. Update `tools.json` with tool schema
5. Create comprehensive documentation

### Tool Testing
- Unit tests for individual tool functions
- Integration tests for tool interactions
- Error condition testing
- Performance benchmarking

## Support

For issues with specific tools, refer to their individual documentation:

- [Weather Tool](./weather/README.md)
- [Web Scraping Tool](./scraping/README.md)
- [Notes Management Tool](./notes/README.md)
- [File System Tool](./filesystem/README.md)
- [Tavily Search Tool](./tavily/README.md)
- [Time Tool](./time/README.md)
- [Cash Flow Tool](./cashflow/README.md)
- [Unit Converter Tool](./converter/README.md)
- [Calendar Tool](./calendar/README.md)
- [Python Execution Tool](./python/README.md)

## License

This tools package is part of the TEO application and follows the same licensing terms. 