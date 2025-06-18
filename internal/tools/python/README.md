# Python Execution Tool

A tool for executing Python code dynamically with support for package installation, input handling, and temporary file management.

## Overview

The Python Execution Tool allows you to run Python code dynamically within the application. It creates temporary environments for code execution, supports package installation, and handles input/output operations safely.

## Features

- **Dynamic Code Execution**: Execute Python code on-demand
- **Package Management**: Install Python packages before execution
- **Input Handling**: Provide input data to Python scripts
- **Temporary Environment**: Isolated execution environment
- **Error Handling**: Comprehensive error reporting
- **Timeout Support**: Configurable execution timeouts
- **Safe Execution**: Temporary file system isolation

## Usage

### Parameters

| Parameter | Type | Required | Description | Example |
|-----------|------|----------|-------------|---------|
| `code` | string | Yes | Python code to execute | `print("Hello, World!")` |
| `timeout` | integer | No | Execution timeout in seconds | 30 |
| `input` | string | No | Input data for the script | "user input" |
| `packages` | string | No | Comma-separated package list | "numpy,pandas" |

### Example Usage

#### Basic Python Execution

```json
{
  "code": "print('Hello, World!')\nprint('Python is running!')",
  "timeout": 10
}
```

#### With Package Installation

```json
{
  "code": "import numpy as np\nprint(np.array([1, 2, 3]))",
  "packages": "numpy",
  "timeout": 30
}
```

#### With Input Data

```json
{
  "code": "import sys\ndata = sys.stdin.read()\nprint(f'Received: {data}')",
  "input": "Hello from input!",
  "timeout": 10
}
```

#### Complex Data Processing

```json
{
  "code": "import pandas as pd\nimport json\n\ndata = [{'name': 'John', 'age': 30}, {'name': 'Jane', 'age': 25}]\ndf = pd.DataFrame(data)\nresult = df.to_dict('records')\nprint(json.dumps(result))",
  "packages": "pandas",
  "timeout": 60
}
```

## Implementation Details

### Dependencies

- Standard Go packages only - No external dependencies
- Requires Python 3 to be installed on the system

### Key Functions

- `NewPythonTool()` - Creates new Python tool instance
- `CallTool(arguments string)` - Main function that processes Python execution
- `executePythonCode(code, input string)` - Executes Python code with input

### Execution Process

1. **Temporary Directory Creation**: Creates isolated temp directory
2. **Package Installation**: Installs required packages using pip
3. **Script File Creation**: Writes Python code to temporary file
4. **Code Execution**: Runs Python script with optional input
5. **Output Capture**: Captures stdout and stderr
6. **Cleanup**: Removes temporary files and directory

### File Management

- **Temporary Directory**: `os.MkdirTemp("", "python-exec-*")`
- **Script File**: `script.py` in temporary directory
- **Automatic Cleanup**: `defer os.RemoveAll(tempDir)`

## Security Considerations

### Execution Safety

- **Temporary Environment**: Code runs in isolated temporary directory
- **Automatic Cleanup**: Temporary files are automatically removed
- **No Persistent Storage**: No permanent file system changes
- **Input Validation**: Validates all input parameters

### Package Installation

- **Controlled Installation**: Only specified packages are installed
- **Temporary Environment**: Packages installed in temp directory only
- **No System-wide Changes**: No permanent package installations

### Code Execution

- **Process Isolation**: Each execution runs in separate process
- **Output Capture**: Captures and returns all output
- **Error Handling**: Comprehensive error reporting
- **Timeout Protection**: Prevents infinite loops

## Error Handling

### Common Errors

#### Package Installation Failure

```
Error installing package numpy: exit status 1
```

#### Code Execution Error

```
Error executing Python code: exit status 1
Output: Traceback (most recent call last):
  File "script.py", line 1, in <module>
    print(undefined_variable)
NameError: name 'undefined_variable' is not defined
```

#### Invalid Arguments

```
Error parsing arguments: unexpected end of JSON input
```

#### Temporary Directory Creation

```
Error creating temp directory: permission denied
```

## Use Cases

### Data Processing

- **Data Analysis**: Run pandas/numpy for data manipulation
- **JSON Processing**: Parse and transform JSON data
- **Text Processing**: String manipulation and text analysis
- **Mathematical Calculations**: Complex mathematical operations

### API Integration

- **HTTP Requests**: Make API calls using requests library
- **Data Fetching**: Retrieve data from external sources
- **Web Scraping**: Extract data from websites

### File Operations

- **File Processing**: Read, write, and manipulate files
- **Image Processing**: Handle images with PIL/Pillow
- **CSV/Excel Processing**: Work with spreadsheet data

### Machine Learning

- **Model Training**: Train simple ML models
- **Data Preprocessing**: Prepare data for analysis
- **Prediction**: Make predictions using trained models

## Best Practices

### Code Safety

- Validate all input data
- Handle exceptions gracefully
- Use appropriate timeouts
- Avoid infinite loops
- Clean up resources properly

### Package Management

- Only install necessary packages
- Use specific package versions when needed
- Consider package size and installation time
- Test package compatibility

### Performance

- Keep code execution time reasonable
- Use efficient algorithms
- Avoid memory-intensive operations
- Consider timeout limits

## Limitations

### System Requirements

- Python 3 must be installed on the system
- pip must be available for package installation
- Sufficient disk space for temporary files
- Adequate memory for code execution

### Execution Constraints

- Temporary execution environment only
- No persistent file system access
- Limited execution time (timeout)
- No network access (unless explicitly allowed)
- No system-level operations

### Package Limitations

- Only packages available via pip
- Installation time affects execution
- Package conflicts possible
- Version compatibility issues

## Examples

### Mathematical Calculations

```json
{
  "code": "import math\n\n# Calculate area of circle\nradius = 5\narea = math.pi * radius ** 2\nprint(f'Area of circle with radius {radius}: {area:.2f}')",
  "timeout": 10
}
```

### Data Analysis

```json
{
  "code": "import pandas as pd\n\n# Create sample data\ndata = {'Name': ['John', 'Jane', 'Bob'], 'Age': [25, 30, 35], 'City': ['NYC', 'LA', 'Chicago']}\ndf = pd.DataFrame(data)\n\n# Filter data\nfiltered = df[df['Age'] > 28]\nprint(filtered.to_string())",
  "packages": "pandas",
  "timeout": 30
}
```

### File Processing

```json
{
  "code": "import json\n\n# Process JSON data\ndata = {'items': [1, 2, 3, 4, 5]}\nresult = {\n    'count': len(data['items']),\n    'sum': sum(data['items']),\n    'average': sum(data['items']) / len(data['items'])\n}\nprint(json.dumps(result, indent=2))",
  "timeout": 10
}
```

### Error Handling Example

```json
{
  "code": "try:\n    result = 10 / 0\nexcept ZeroDivisionError:\n    print('Error: Division by zero')\nexcept Exception as e:\n    print(f'Unexpected error: {e}')\nelse:\n    print(f'Result: {result}')",
  "timeout": 10
}
```
