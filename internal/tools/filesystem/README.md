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
- **Directory Tree**: JSON tree view of directory structure

## Security Model

### Allowed Directories

By default, the tool is restricted to: `~/teo_home` (user's home directory + "teo_home")

- All operations are validated against allowed directories
- Path traversal attacks are prevented
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

## Example Usage

### Read a File

```json
{
  "tool_name": "read_file",
  "path": "~/teo_home/document.txt"
}
```

### Write a File

```json
{
  "tool_name": "write_file",
  "path": "~/teo_home/new_file.txt",
  "content": "Hello, World!"
}
```

### Edit File Lines

```json
{
  "tool_name": "edit_file",
  "path": "~/teo_home/config.txt",
  "edit_start_line": 5,
  "edit_end_line": 7,
  "edit_new_content": "new line 5\nnew line 6\nnew line 7"
}
```

### Search Files

```json
{
  "tool_name": "search_files",
  "path": "~/teo_home",
  "pattern": "*.txt"
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
- No symbolic link handling
- File size limited by system memory

## Best Practices

- Always validate paths before operations
- Use descriptive file and directory names
- Regular backups of important data
- Monitor allowed directory usage
- Handle errors gracefully in applications
