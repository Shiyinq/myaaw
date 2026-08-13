package filesystem

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"myaaw/internal/config"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"myaaw/internal/agent/tools"
)

const toolSchema = `{
    "type": "function",
    "function": {
        "name": "filesystem",
        "description": "Manages files and directories within allowed locations. You can combine these functions to perform complex tasks. All paths must be within permitted directories.\nAvailable functions:\n- \"read_file\": Reads specific lines of a specified file. Requires 'start_line' and 'end_line'.\n- \"read_multiple_files\": Reads contents of several files at once. Provide paths as a JSON array or comma-separated string for the 'path' argument.\n- \"write_file\": Creates a new file or overwrites an existing one with provided content. Use with caution.\n- \"edit_file\": Performs line-based edits on a text file. Specify start/end lines and new content. Returns a diff.\n- \"create_directory\": Creates a new directory. Can create nested directories. Silent if directory already exists.\n- \"list_directory\": Lists all files and subdirectories in a specified directory, marking type (FILE/DIR). Output is truncated at 32KB.\n- \"directory_tree\": Provides a recursive JSON tree view of files and directories from a starting path. Supports 'depth' limit and is truncated at 32KB.\n- \"move_file\": Moves or renames files/directories. Fails if destination exists.\n- \"search_files\": Recursively searches for files/directories matching a case-insensitive pattern. Output is truncated at 32KB.\n- \"grep_search\": Searches for specific text or regex patterns inside files within a directory. Output is truncated at 32KB.\n- \"get_file_info\": Retrieves detailed metadata (size, type, modified time, permissions) for a file or directory.\n- \"list_allowed_directories\": Shows the list of directories this tool can access.\n- \"delete_path\": Deletes a specified file or directory. Use the 'delete_recursive' boolean parameter to delete non-empty directories.\n\nConsider chaining ini operations. For example: list files with ` + "`list_directory`" + `, read one with ` + "`read_file`" + `, modify it with ` + "`edit_file`" + `, then verify with ` + "`get_file_info`" + `. Or, create a directory structure with ` + "`create_directory`" + ` then populate it using ` + "`write_file`" + ` or ` + "`move_file`" + `.",
        "parameters": {
            "type": "object",
            "properties": {
                "tool_name": {
                    "type": "string",
                    "description": "The specific file system function to execute.",
                    "enum": [
                        "read_file",
                        "read_multiple_files",
                        "write_file",
                        "edit_file",
                        "create_directory",
                        "list_directory",
                        "directory_tree",
                        "move_file",
                        "search_files",
                        "grep_search",
                        "get_file_info",
                        "list_allowed_directories",
                        "delete_path"
                    ]
                },
                "path": {
                    "type": "string",
                    "description": "The primary path for the operation (e.g., file to read, directory to list, file to edit for edit_file, path to delete for delete_path). Required for list_directory and directory_tree. For read_multiple_files, this can be a JSON array of paths or a comma-separated string of paths."
                },
                "content": {
                    "type": "string",
                    "description": "Content to be written to a file (used by write_file)."
                },
                "old_path": {
                    "type": "string",
                    "description": "The source path for a move operation (for move_file)."
                },
                "new_path": {
                    "type": "string",
                    "description": "The destination path for a move operation (for move_file)."
                },
                "pattern": {
                    "type": "string",
                    "description": "The search pattern for search_files."
                },
                "edit_start_line": {
                    "type": "integer",
                    "description": "The 1-indexed line number where the edit should begin. Required for edit_file."
                },
                "edit_end_line": {
                    "type": "integer",
                    "description": "Optional. The 1-indexed line number where the edit should end (inclusive). If not provided or less than edit_start_line, only the single line at edit_start_line is targeted for replacement by edit_new_content."
                },
                "edit_new_content": {
                    "type": "string",
                    "description": "The new content to replace the specified line(s). Required for edit_file. Multiple lines can be separated by \n."
                },
                "delete_recursive": {
                    "type": "boolean",
                    "description": "Optional. If true, allows recursive deletion of directories and their contents. Defaults to false. Used by delete_path."
                },
                "start_line": {
                    "type": "integer",
                    "description": "The 1-indexed line number where the reading should begin. Required for read_file."
                },
                "end_line": {
                    "type": "integer",
                    "description": "The 1-indexed line number where the reading should end (inclusive). Required for read_file."
                },
                "regex": {
                    "type": "boolean",
                    "description": "If true, treats the pattern as a regular expression. Used by grep_search."
                },
                "depth": {
                    "type": "integer",
                    "description": "The maximum depth for recursive directory tree view. Default is 2. Used by directory_tree."
                }
            },
            "required": [
                "tool_name"
            ]
        }
    }
}`

var (
	allowedDirectories []string
	defaultBaseDir     string
)

func init() {
	tools.RegisterBuiltin("filesystem", NewFileSystemTool())

	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("Warning: Could not get current working directory: %v", err)
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		myaawHome := filepath.Join(homeDir, ".myaaw", "home")
		myaawSkills := filepath.Join(homeDir, ".myaaw", "skills")

		defaultBaseDir = myaawHome
		allowedDirectories = append(allowedDirectories, myaawHome, myaawSkills)

		if config.Verbose {
			log.Printf("FileSystemTool: Allowed directories set to: %v", allowedDirectories)
		}
	} else {
		if config.Verbose {
			log.Printf("Error: Could not determine home directory, sandbox limited.")
		}
	}

	// Smart CWD Detection:
	// Only treat CWD as project root if it looks like one (has go.mod, .git, or .env)
	isProject := false
	if cwd != "" {
		markers := []string{"go.mod", ".git", ".env"}
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(cwd, marker)); err == nil {
				isProject = true
				break
			}
		}
	}

	if isProject {
		defaultBaseDir = cwd
		allowedDirectories = append(allowedDirectories, cwd)
		if config.Verbose {
			log.Printf("FileSystemTool: Project detected at %s. Added to allowed paths.", cwd)
		}
	} else {
		if config.Verbose {
			log.Printf("FileSystemTool: Current directory %s does not look like a project. Sandbox restricted to %s", cwd, defaultBaseDir)
		}
	}
}

// TODO: Make allowedDirectories configurable (e.g., via environment variable, config file)

type FileSystemTool struct{}

func NewFileSystemTool() *FileSystemTool {
	return &FileSystemTool{}
}

func (t *FileSystemTool) ToolDefinition() []byte {
	return []byte(toolSchema)
}

type FileSystemArgs struct {
	ToolName string `json:"tool_name"` // To distinguish which file system function to call
	Path     string `json:"path"`
	OldPath  string `json:"old_path"`
	NewPath  string `json:"new_path"`
	Content  string `json:"content"`
	Pattern  string `json:"pattern"`
	// Add other arguments from the issue description as needed for different tools
	// For example, for edit_file:
	EditStartLine   int    `json:"edit_start_line"`  // Line number to start editing (1-indexed)
	EditEndLine     int    `json:"edit_end_line"`    // Line number to end editing (1-indexed, inclusive, optional)
	EditNewContent  string `json:"edit_new_content"` // New content to replace/insert
	DeleteRecursive bool   `json:"delete_recursive"` // Flag for recursive deletion, used by delete_path
	StartLine       int    `json:"start_line"`       // Optional start line for read_file
	EndLine         int    `json:"end_line"`         // Optional end line for read_file
	Regex           bool   `json:"regex"`            // Flag for grep_search to use regex pattern
	Depth           int    `json:"depth"`            // Depth for directory_tree
}

func expandPath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, path[2:]), nil
	}
	// If path is relative, join it with the defaultBaseDir (Smart CWD)
	if !filepath.IsAbs(path) {
		return filepath.Join(defaultBaseDir, path), nil
	}
	return path, nil
}

func isAllowed(path string) (string, error) {
	expandedPath, err := expandPath(path)
	if err != nil {
		return "", fmt.Errorf("error expanding path: %w", err)
	}

	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return "", fmt.Errorf("error getting absolute path: %w", err)
	}

	// Evaluate symlinks to prevent path traversal
	evaluatedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If it doesn't exist yet, we still check the intended absolute path
			evaluatedPath = absPath
		} else {
			return "", fmt.Errorf("error evaluating symlinks: %w", err)
		}
	}

	for _, allowedDir := range allowedDirectories {
		// Also evaluate allowedDir symlinks for consistency
		absAllowedDir, err := filepath.Abs(allowedDir)
		if err != nil {
			continue
		}
		evalAllowedDir, err := filepath.EvalSymlinks(absAllowedDir)
		if err == nil {
			absAllowedDir = evalAllowedDir
		}

		if strings.HasPrefix(evaluatedPath, absAllowedDir) {
			return evaluatedPath, nil
		}
	}
	return "", fmt.Errorf("path '%s' (resolved to '%s') is not within allowed directories", path, evaluatedPath)
}

func (f *FileSystemTool) CallTool(arguments string, ctx *tools.ToolsContext) string {
	var args FileSystemArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	// Security check for all paths involved in the operation
	pathsToCheck := []string{args.Path, args.OldPath, args.NewPath}
	for _, p := range pathsToCheck {
		if p != "" { // Only check non-empty paths
			if _, err := isAllowed(p); err != nil {
				return fmt.Sprintf("Security error: %v", err)
			}
		}
	}

	// Based on args.ToolName, call the appropriate private method.
	// For example:
	switch args.ToolName {
	case "read_file":
		if args.Path == "" || args.StartLine == 0 || args.EndLine == 0 {
			return "Error: For read_file, 'path', 'start_line', and 'end_line' are required arguments."
		}
		return f.readFile(args.Path, args.StartLine, args.EndLine)
	case "read_multiple_files":
		// Assuming Path might be a comma-separated list of files or JSON array string
		var multiFilePaths []string
		if err := json.Unmarshal([]byte(args.Path), &multiFilePaths); err != nil {
			// Fallback for comma-separated if JSON unmarshal fails
			multiFilePaths = strings.Split(args.Path, ",")
		}
		return f.readMultipleFiles(multiFilePaths)
	case "write_file":
		return f.writeFile(args.Path, args.Content)
	case "edit_file":
		// Ensure required args for edit_file are present, e.g., Path, EditStartLine, EditNewContent.
		// EditEndLine is optional, defaults to EditStartLine if not provided or < EditStartLine.
		if args.Path == "" || args.EditStartLine == 0 || args.EditNewContent == "" {
			return "Error: For edit_file, 'path', 'edit_start_line', and 'edit_new_content' are required arguments."
		}
		return f.editFile(args.Path, args.EditStartLine, args.EditEndLine, args.EditNewContent)
	case "create_directory":
		return f.createDirectory(args.Path)
	case "list_directory":
		return f.truncateAndLog(f.listDirectory(args.Path), "filesystem-list")
	case "directory_tree":
		return f.truncateAndLog(f.directoryTree(args.Path, args.Depth), "filesystem-tree")
	case "move_file":
		return f.moveFile(args.OldPath, args.NewPath)
	case "search_files":
		// Assuming args.Path is the directory to search in and args.Pattern is the search pattern
		return f.truncateAndLog(f.searchFiles(args.Path, args.Pattern), "filesystem-search")
	case "grep_search":
		// Search for content within files
		if args.Path == "" || args.Pattern == "" {
			return "Error: For grep_search, 'path' and 'pattern' are required arguments."
		}
		return f.truncateAndLog(f.grepSearch(args.Path, args.Pattern, args.Regex), "filesystem-grep")
	case "get_file_info":
		return f.getFileInfo(args.Path)
	case "list_allowed_directories":
		return f.listAllowedDirectories()
	case "delete_path":
		if args.Path == "" {
			return "Error: For delete_path, 'path' is a required argument."
		}
		// args.DeleteRecursive defaults to false if not provided, which is fine.
		return f.deletePath(args.Path, args.DeleteRecursive)
	default:
		return fmt.Sprintf("Error: tool_name '%s' not recognized within FileSystemTool.", args.ToolName)
	}
}

// truncateAndLog truncates output if it exceeds 32KB and saves to a log file
func (f *FileSystemTool) truncateAndLog(output string, prefix string) string {
	const maxOutputSize = 32 * 1024
	if len(output) > maxOutputSize {
		logPath := "unknown"
		if homeDir, err := os.UserHomeDir(); err == nil {
			logsDir := filepath.Join(homeDir, ".myaaw", "home", ".logs")
			os.MkdirAll(logsDir, 0755)
			if tempFile, err := os.CreateTemp(logsDir, fmt.Sprintf("%s-*.log", prefix)); err == nil {
				tempFile.WriteString(output)
				logPath = tempFile.Name()
				tempFile.Close()
			}
		}
		return output[:maxOutputSize] + fmt.Sprintf("\n\n... [Output truncated because it exceeded 32KB limit. Full output saved to: %s. Use 'read_file' tool with start_line and end_line to read it.] ...", logPath)
	}
	return output
}

// Implement private methods for each file system operation here.
// Example for readFile:
func (f *FileSystemTool) readFile(path string, startLine, endLine int) string {
	absPath, err := isAllowed(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("Error reading file %s: %v", path, err)
	}

	// Detect binary
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "text/") && contentType != "application/json" && !strings.Contains(contentType, "xml") {
		encodedStr := base64.StdEncoding.EncodeToString(data)
		return fmt.Sprintf("[[BINARY FILE DETECTED]]\nFormat: Base64\nMIME Type: %s\nContent:\n%s", contentType, encodedStr)
	}

	strData := string(data)
	lines := strings.Split(strData, "\n")

	startIndex := startLine - 1
	endIndex := endLine - 1

	if startIndex < 0 || startIndex >= len(lines) || endIndex < startIndex || endIndex >= len(lines) {
		return fmt.Sprintf("Error: Line range [%d, %d] is invalid or out of bounds for file with %d lines.", startLine, endLine, len(lines))
	}

	return strings.Join(lines[startIndex:endIndex+1], "\n")
}

func (f *FileSystemTool) readMultipleFiles(paths []string) string {
	type fileContent struct {
		Path    string `json:"path"`
		Content string `json:"content,omitempty"`
		Error   string `json:"error,omitempty"`
	}
	var results []fileContent

	for _, path := range paths {
		trimmedPath := strings.TrimSpace(path)
		absPath, err := isAllowed(trimmedPath)
		if err != nil {
			results = append(results, fileContent{Path: trimmedPath, Error: err.Error()})
			continue
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			results = append(results, fileContent{Path: trimmedPath, Error: fmt.Sprintf("Error reading file: %v", err)})
		} else {
			results = append(results, fileContent{Path: trimmedPath, Content: string(data)})
		}
	}
	resultBytes, err := json.Marshal(results)
	if err != nil {
		return fmt.Sprintf("Error marshalling results: %v", err)
	}
	return string(resultBytes)
}

func (f *FileSystemTool) writeFile(path string, content string) string {
	absPath, err := isAllowed(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	err = os.WriteFile(absPath, []byte(content), 0644) // Default permissions
	if err != nil {
		return fmt.Sprintf("Error writing file %s: %v", path, err)
	}
	return fmt.Sprintf("File %s written successfully.", path)
}

func (f *FileSystemTool) createDirectory(path string) string {
	absPath, err := isAllowed(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	err = os.MkdirAll(absPath, os.ModePerm) // os.ModePerm (0777) is often used, but consider more restrictive permissions
	if err != nil {
		return fmt.Sprintf("Error creating directory %s: %v", path, err)
	}
	return fmt.Sprintf("Directory %s created successfully or already exists.", path)
}

func (f *FileSystemTool) listDirectory(path string) string {
	if path == "" {
		return "Error: 'path' argument is required for list_directory."
	}
	absPath, err := isAllowed(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return fmt.Sprintf("Error listing directory %s: %v", path, err)
	}
	var result []string
	for _, entry := range entries {
		prefix := "[FILE]"
		if entry.IsDir() {
			prefix = "[DIR]"
		}
		result = append(result, fmt.Sprintf("%s %s", prefix, entry.Name()))
	}
	return strings.Join(result, "\n")
}

type DirEntry struct {
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Children []DirEntry `json:"children,omitempty"`
}

func (f *FileSystemTool) directoryTree(basePath string, depth int) string {
	if basePath == "" {
		return "Error: 'path' argument is required for directory_tree."
	}
	absBasePath, err := isAllowed(basePath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	if depth <= 0 {
		depth = 2 // Default depth
	}

	nodeCount := 0
	const maxNodes = 1000

	var buildTree func(currentPath string, currentDepth int) (DirEntry, error)
	buildTree = func(currentPath string, currentDepth int) (DirEntry, error) {
		if nodeCount >= maxNodes {
			return DirEntry{Name: filepath.Base(currentPath), Type: "limit_reached"}, nil
		}
		nodeCount++

		info, err := os.Stat(currentPath)
		if err != nil {
			return DirEntry{}, fmt.Errorf("error stating path %s: %w", currentPath, err)
		}

		entry := DirEntry{
			Name: filepath.Base(currentPath),
		}

		if info.IsDir() {
			entry.Type = "directory"

			if currentDepth >= depth {
				return entry, nil // Stop at depth
			}

			entry.Children = []DirEntry{}

			files, err := os.ReadDir(currentPath)
			if err != nil {
				return DirEntry{}, fmt.Errorf("error reading directory %s: %w", currentPath, err)
			}

			for _, file := range files {
				if nodeCount >= maxNodes {
					entry.Children = append(entry.Children, DirEntry{Name: "... [Node limit reached]", Type: "limit_reached"})
					break
				}

				childPath := filepath.Join(currentPath, file.Name())
				childEntry, err := buildTree(childPath, currentDepth+1)
				if err != nil {
					fmt.Printf("Skipping child %s due to error: %v\n", childPath, err)
					continue
				}
				entry.Children = append(entry.Children, childEntry)
			}
		} else {
			entry.Type = "file"
		}
		return entry, nil
	}

	rootEntry, err := buildTree(absBasePath, 0)
	if err != nil {
		return fmt.Sprintf("Error building directory tree for %s: %v", basePath, err)
	}

	var dataToMarshal interface{} = rootEntry
	jsonData, err := json.MarshalIndent(dataToMarshal, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshalling directory tree to JSON: %v", err)
	}

	result := string(jsonData)
	if nodeCount >= maxNodes {
		result += "\n\n... [Warning: Directory tree was truncated due to node limit (1000)] ..."
	}
	return result
}

func (f *FileSystemTool) moveFile(oldPath, newPath string) string {
	absOldPath, err := isAllowed(oldPath)
	if err != nil {
		return fmt.Sprintf("Error (source path): %v", err)
	}
	absNewPath, err := isAllowed(newPath) // Also check destination
	if err != nil {
		return fmt.Sprintf("Error (destination path): %v", err)
	}

	// Check if destination exists
	if _, err := os.Stat(absNewPath); err == nil {
		return fmt.Sprintf("Error moving file: destination %s already exists.", newPath)
	} else if !os.IsNotExist(err) {
		// Another error occurred with stat on newPath
		return fmt.Sprintf("Error checking destination path %s: %v", newPath, err)
	}

	err = os.Rename(absOldPath, absNewPath)
	if err != nil {
		return fmt.Sprintf("Error moving file from %s to %s: %v", oldPath, newPath, err)
	}
	return fmt.Sprintf("File moved successfully from %s to %s.", oldPath, newPath)
}

func (f *FileSystemTool) searchFiles(dirPath, pattern string) string {
	absDirPath, err := isAllowed(dirPath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	var foundPaths []string
	err = filepath.WalkDir(absDirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Log or handle error during walk, e.g. permission denied on a subdirectory
			fmt.Printf("Warning: error walking path %s: %v. Skipping.\n", path, err)
			return nil // Continue walking
		}
		// Perform case-insensitive partial match on the name (file or directory)
		if strings.Contains(strings.ToLower(d.Name()), strings.ToLower(pattern)) {
			foundPaths = append(foundPaths, path)
		}
		return nil
	})

	if err != nil {
		// This error would be from filepath.WalkDir itself if it couldn't start
		return fmt.Sprintf("Error searching files in %s: %v", dirPath, err)
	}

	if len(foundPaths) == 0 {
		return fmt.Sprintf("No files or directories found matching pattern '%s' in %s.", pattern, dirPath)
	}

	resultBytes, err := json.Marshal(foundPaths)
	if err != nil {
		return fmt.Sprintf("Error marshalling search results: %v", err)
	}
	return string(resultBytes)
}

func (f *FileSystemTool) grepSearch(dirPath, pattern string, useRegex bool) string {
	absDirPath, err := isAllowed(dirPath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	var compiledRegex *regexp.Regexp
	if useRegex {
		var errRegex error
		compiledRegex, errRegex = regexp.Compile(pattern)
		if errRegex != nil {
			return fmt.Sprintf("Error compiling regex pattern: %v", errRegex)
		}
	} else {
		pattern = strings.ToLower(pattern)
	}

	var results []string

	err = filepath.WalkDir(absDirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		// Quick check before reading the full file to avoid opening binaries
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		// Read first 512 bytes for content type detection
		buffer := make([]byte, 512)
		n, _ := file.Read(buffer)
		if n > 0 {
			contentType := http.DetectContentType(buffer[:n])
			if !strings.HasPrefix(contentType, "text/") && contentType != "application/json" && !strings.Contains(contentType, "xml") {
				return nil // Skip binary files
			}
		}

		// Reset file pointer to beginning
		file.Seek(0, 0)

		scanner := bufio.NewScanner(file)
		lineNumber := 1
		for scanner.Scan() {
			line := scanner.Text()
			match := false
			if useRegex {
				match = compiledRegex.MatchString(line)
			} else {
				match = strings.Contains(strings.ToLower(line), pattern)
			}

			if match {
				// Relativize path for nicer output
				relPath := path
				if relative, err := filepath.Rel(absDirPath, path); err == nil {
					relPath = relative
				}
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, lineNumber, strings.TrimSpace(line)))

				// Limit to a reasonable number of overall matches to avoid overwhelming output
				if len(results) > 100 {
					results = append(results, "... [Output truncated due to too many matches (>100)] ...")
					return filepath.SkipAll // Stop walking if we have too many
				}
			}
			lineNumber++
		}
		return nil
	})

	if err != nil && err != filepath.SkipAll {
		return fmt.Sprintf("Error during grep_search in %s: %v", dirPath, err)
	}

	if len(results) == 0 {
		return fmt.Sprintf("No content found matching pattern '%s' in %s.", pattern, dirPath)
	}

	return strings.Join(results, "\n")
}

func (f *FileSystemTool) getFileInfo(path string) string {
	absPath, err := isAllowed(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Sprintf("Error getting file info for %s: %v", path, err)
	}

	fileType := "file"
	if info.IsDir() {
		fileType = "directory"
	}

	// Simplified representation, can be expanded
	fileInfo := map[string]interface{}{
		"name":        info.Name(),
		"size":        info.Size(), // bytes
		"type":        fileType,
		"modified_at": info.ModTime().Format(time.RFC3339),
		"permissions": info.Mode().String(),
	}
	resultBytes, err := json.MarshalIndent(fileInfo, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshalling file info: %v", err)
	}
	return string(resultBytes)
}

func (f *FileSystemTool) listAllowedDirectories() string {
	// Make sure to return a JSON array string as per typical tool outputs
	// if they are expected to be machine-readable.
	// For now, returning a simple string as other messages.
	// Consider if the output should be `{"allowed_directories": ["/path1", "/path2"]}`
	resultBytes, err := json.Marshal(allowedDirectories)
	if err != nil {
		return fmt.Sprintf("Error marshalling allowed directories: %v", err)
	}
	return string(resultBytes)
}

// TODO: Implement edit_file. This is more complex due to line-based operations.
// It might involve reading the file, splitting by lines, making changes, and writing back.
// A git-style diff can be generated by comparing the original and new content line by line,
// or by using a diff library if one is available/allowed.

func (f *FileSystemTool) editFile(path string, startLine int, endLine int, newContent string) string {
	absPath, err := isAllowed(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	originalData, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("Error reading file %s for edit: %v", path, err)
	}
	originalLines := strings.Split(string(originalData), "\n")

	// Convert 1-indexed lines to 0-indexed for slice operations
	startIndex := startLine - 1
	// If endLine is not provided or is less than startLine, assume editing/replacing a single line (or inserting before, depending on interpretation)
	// For "replaces exact line sequences", let's assume endLine defaults to startLine if not provided or invalid.
	endIndex := startLine - 1 // Default to affecting only the startLine if endLine is not specified
	if endLine >= startLine {
		endIndex = endLine - 1
	}

	if startIndex < 0 || startIndex > len(originalLines) { // Allow startIndex == len(originalLines) for appending
		return fmt.Sprintf("Error: Start line %d is out of bounds for file with %d lines.", startLine, len(originalLines))
	}
	if endIndex < 0 || endIndex >= len(originalLines) && startIndex != len(originalLines) { // Allow endIndex for append scenario if startIndex is also at end
		if endIndex == len(originalLines) && startIndex == len(originalLines) { // append case
			// this is fine, effectively an append
		} else {
			return fmt.Sprintf("Error: End line %d is out of bounds for file with %d lines.", endLine, len(originalLines))
		}
	}
	if startIndex > endIndex && startIndex != len(originalLines) { // if startIndex is for append, endIndex is irrelevant if smaller
		return fmt.Sprintf("Error: Start line %d cannot be after end line %d.", startLine, endLine)
	}

	newContentLines := strings.Split(newContent, "\n")

	var modifiedLines []string
	// Add lines before the edit range
	if startIndex > 0 {
		modifiedLines = append(modifiedLines, originalLines[:startIndex]...)
	}

	// Add the new content
	modifiedLines = append(modifiedLines, newContentLines...)

	// Add lines after the edit range
	// If startIndex is for append (i.e. startIndex == len(originalLines)), then originalLines[endIndex+1:] would be empty or panic
	// and originalLines[:startIndex] would contain all original lines.
	if endIndex+1 < len(originalLines) && startIndex <= endIndex {
		modifiedLines = append(modifiedLines, originalLines[endIndex+1:]...)
	} else if startIndex == len(originalLines) {
		// This is an append operation, no lines after original end to add.
	} else if startIndex > endIndex { // This implies replacing a single line, originalLines[startIndex+1:]
		// This case should be handled by endIndex defaulting to startIndex if not specified,
		// so this specific else-if might be redundant if logic is clean.
		// If replacing single line at startIndex, then originalLines[startIndex+1:] are the ones to add after newContent.
		if startIndex+1 < len(originalLines) {
			modifiedLines = append(modifiedLines, originalLines[startIndex+1:]...)
		}
	}

	finalContent := strings.Join(modifiedLines, "\n")
	err = os.WriteFile(absPath, []byte(finalContent), 0644)
	if err != nil {
		return fmt.Sprintf("Error writing updated content to file %s: %v", path, err)
	}

	// Generate git-style diff (simple version)
	var diff []string
	// For simplicity, this diff will be a full before/after rather than line-by-line additions/deletions marks
	// A true line-by-line diff is more complex.
	// Let's do a basic line diff based on what was replaced.

	// i, j := 0, 0 // These variables are declared but not used in the provided code.
	// beforeLines := originalLines // Declared but not used
	// afterLines := modifiedLines // Declared but not used

	// This is a simplified diff, a proper one would use a LCS algorithm.
	// For now, just show removed and added blocks based on the edit.
	diff = append(diff, fmt.Sprintf("--- a/%s", path))
	diff = append(diff, fmt.Sprintf("+++ b/%s", path))

	// Show lines that were replaced/removed
	for k := startIndex; k <= endIndex && k < len(originalLines); k++ {
		diff = append(diff, fmt.Sprintf("-%s", originalLines[k]))
	}
	// Show new lines that were added
	for _, line := range newContentLines {
		diff = append(diff, fmt.Sprintf("+%s", line))
	}

	// If the diff is very large, this might be too verbose.
	// A more sophisticated diff would show context lines.

	return fmt.Sprintf("File %s edited successfully.\nDiff:\n%s", path, strings.Join(diff, "\n"))
}

func (f *FileSystemTool) deletePath(path string, recursive bool) string {
	absPath, err := isAllowed(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		return fmt.Sprintf("Error: Path %s does not exist.", path)
	}
	if err != nil {
		return fmt.Sprintf("Error stating path %s: %v", path, err)
	}

	if recursive {
		err = os.RemoveAll(absPath)
		if err != nil {
			return fmt.Sprintf("Error recursively deleting %s: %v", path, err)
		}
		return fmt.Sprintf("Path %s recursively deleted successfully.", path)
	} else {
		// Check if it's a directory and not empty (os.Remove will fail, but we can give a better message)
		if info.IsDir() {
			dirEntries, _ := os.ReadDir(absPath)
			if len(dirEntries) > 0 {
				return fmt.Sprintf("Error: Directory %s is not empty. Use recursive delete if intended.", path)
			}
		}
		err = os.Remove(absPath)
		if err != nil {
			// Error might be because it's a non-empty directory and recursive was false
			// Or other permission issues.
			return fmt.Sprintf("Error deleting %s: %v. If it is a non-empty directory, recursive delete might be needed.", path, err)
		}
		return fmt.Sprintf("Path %s deleted successfully.", path)
	}
}
