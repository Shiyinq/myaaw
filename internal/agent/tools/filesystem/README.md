# File System Management Tool

A comprehensive tool for managing files and directories with security restrictions and multiple file operations.

## Overview

The File System Tool provides a wide range of file and directory management capabilities while maintaining security through allowed directory restrictions. It supports reading, writing, editing, moving, searching, and deleting files and directories.

## Features

- **File Operations**: Read, write, edit, move, delete files
- **Directory Operations**: Create, list, search, delete directories
- **Security**: Restricted to allowed directories only
- **Multiple File Reading**: Read multiple files at once
- **File Editing**: Line-based file editing with diff output
- **Search Capabilities**: Recursive file and directory search
- **File Information**: Detailed metadata retrieval
- **Directory Tree**: JSON tree view with `depth` and node limits
- **Binary Detection**: Automatically detects and Base64 encodes binary files
- **Grep Search**: Search for specific content strings or regex patterns inside files
- **Output Truncation**: Large outputs (>32KB) are automatically saved to log files


## Security Model

### Allowed Directories

By default, the tool is restricted to: `~/myaaw_home` (user's home directory + "myaaw_home")

- All operations are validated against allowed directories
- **Symlink Evaluation**: Symbolic links are followed and validated to prevent escaping the sandbox
- Absolute path resolution ensures security


## Available Operations

| Operation | Description | Required Parameters |
|-----------|-------------|-------------------|
| `read_file` | Read single file content | `path` |
| `read_multiple_files` | Read multiple files | `path` (JSON array or comma-separated) |
| `write_file` | Create/overwrite file | `path`, `content` |
| `edit_file` | Edit specific lines in file | `path`, `edit_start_line`, `edit_new_content` |
| `create_directory` | Create directory | `path` |
| `list_directory` | List directory contents | `path` |
| `directory_tree` | Get recursive directory tree | `path` |
| `move_file` | Move/rename files/directories | `old_path`, `new_path` |
| `search_files` | Search files by pattern | `path`, `pattern` |
| `grep_search` | Search content inside files | `path`, `pattern` |
| `get_file_info` | Get file metadata | `path` |
| `list_allowed_directories` | Show allowed directories | None |
| `delete_path` | Delete file/directory | `path` |

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tool_name` | string | Yes | Operation to perform |
| `path` | string | Conditional | File/directory path |
| `content` | string | Conditional | Content to write |
| `old_path` | string | Conditional | Source path for move |
| `new_path` | string | Conditional | Destination path for move |
| `pattern` | string | Conditional | Search pattern |
| `edit_start_line` | integer | Conditional | Start line for editing (1-indexed) |
| `edit_end_line` | integer | Conditional | End line for editing (optional) |
| `edit_new_content` | string | Conditional | New content for editing |
| `delete_recursive` | boolean | Conditional | Enable recursive deletion |
| `start_line` | integer | Yes (for read_file) | Starting line for reading (1-indexed) |
| `end_line` | integer | Yes (for read_file) | Ending line for reading (1-indexed) |
| `regex` | boolean | No | Use regex for grep_search |
| `depth` | integer | No | Max depth for directory_tree (default: 2) |

## Example Usage

### Read a File (Mandatory line range)

```json
{
  "tool_name": "read_file",
  "path": "~/myaaw_home/document.txt",
  "start_line": 1,
  "end_line": 100
}
```


### Write a File

```json
{
  "tool_name": "write_file",
  "path": "~/myaaw_home/new_file.txt",
  "content": "Hello, World!"
}
```

### Edit File Lines

```json
{
  "tool_name": "edit_file",
  "path": "~/myaaw_home/config.txt",
  "edit_start_line": 5,
  "edit_end_line": 7,
  "edit_new_content": "new line 5\nnew line 6\nnew line 7"
}
```

```json
{
  "tool_name": "search_files",
  "path": "~/myaaw_home",
  "pattern": "*.txt"
}
```

### Grep Search Content

```json
{
  "tool_name": "grep_search",
  "path": "~/myaaw_home",
  "pattern": "TODO:",
  "regex": false
}
```


## Implementation Details

### Key Functions

- `NewFileSystemTool()` - Creates new filesystem tool instance
- `CallTool(arguments string)` - Main function that processes operations
- `isAllowed(path string)` - Validates path security
- `readFile(path string)` - Reads single file
- `writeFile(path, content string)` - Writes file content
- `editFile(path, startLine, endLine, newContent string)` - Edits file lines
- `searchFiles(dirPath, pattern string)` - Searches files recursively

### Security Features

1. **Path Validation**: All paths checked against allowed directories
2. **Absolute Path Resolution**: Prevents path traversal attacks
3. **Directory Restrictions**: Operations limited to safe directories
4. **Input Sanitization**: Validates all input parameters

### File Operations

- **Reading**: Supports single and multiple file reading
- **Writing**: Creates new files or overwrites existing ones
- **Editing**: Line-based editing with diff output
- **Moving**: Rename or move files/directories
- **Deleting**: Safe deletion with recursive option

### Directory Operations

- **Creation**: Creates nested directories automatically
- **Listing**: Shows files and subdirectories with types
- **Tree View**: Recursive JSON structure of directories
- **Searching**: Pattern-based file and directory search

## Error Handling

- Security violations (unauthorized paths)
- File system errors (permissions, not found)
- Invalid parameters
- Operation-specific errors
- JSON parsing errors

## Response Formats

### File Content

Returns raw file content as string

### Directory Listing

```json
[
  {
    "name": "file.txt",
    "type": "FILE"
  },
  {
    "name": "folder",
    "type": "DIR"
  }
]
```

### File Information

```json
{
  "name": "file.txt",
  "size": 1024,
  "type": "FILE",
  "modified": "2024-01-01T10:00:00Z",
  "permissions": "0644"
}
```

## Limitations

- Restricted to allowed directories only
- No network file system support
- No file compression/decompression
- No file encryption/decryption
- No symbolic link handling (except for security validation)
- File size limited by system memory
- **Output Limit**: Outputs are truncated at 32KB; full results are saved to `~/.myaaw/home/.logs/`
- **Node Limit**: `directory_tree` is limited to 1000 nodes to prevent oversized JSON

## Best Practices

- Always validate paths before operations
- Use descriptive file and directory names
- Regular backups of important data
- Monitor allowed directory usage
- Handle errors gracefully in applications
