# Myaaw Tools Documentation

A comprehensive collection of tools for the Myaaw application, providing various functionalities for data management, web services, file operations, and more.

## Overview

The Myaaw Tools package contains a set of specialized tools that can be used within the Myaaw application. Each tool is designed to handle specific tasks and provides a standardized interface for integration.

## Available Tools

### 1. [File System Tool](./filesystem/README.md)

**Function**: `filesystem`

- Comprehensive file and directory management
- Security-restricted to allowed directories
- Multiple operations: read, write, edit, move, delete, search

### 2. [Python Execution Tool](./python/README.md)

**Function**: `execute_python`

- Dynamic Python code execution
- Package installation support
- Temporary environment with input/output handling

### 3. [Bash Execution Tool](./bash/README.md)

**Function**: `bash`

- Execute bash commands
- Manage files and directories
- Run scripts and utilities

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
    "bash":           bash.NewBashTool(),
    "filesystem":     filesystem.NewFileSystemTool(),
    "execute_python": python.NewPythonTool(),
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

- **File System Tool**: Restricted to `~/myaaw_home` directory
- **Python Tool**: Temporary execution environment
- **All Tools**: Input validation and error handling

## Usage Examples

### Basic Tool Call

```go
result := NewTools("bash", `{"command": "echo 'Hello World'"}`)
```

### Tool with Complex Parameters

```go
result := NewTools("execute_python", `{
    "code": "print('Hello World')",
    "packages": ["requests"]
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

1. Create a new directory in `internal/agent/tools/`
2. Implement the `ToolsFactory` interface
3. Add tool registration in `tools.go`
4. Update `tools.json` with tool schema
5. Create comprehensive documentation

### Tool Testing

- Unit tests for individual tool functions
- Integration tests for tool interactions
- Error condition testing
- Performance benchmarking

## License

This tools package is part of the Myaaw application and follows the same licensing terms.
