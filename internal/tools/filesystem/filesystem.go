package filesystem

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var allowedDirectories []string

func init() {
	// Add current working directory to allowed paths to support 'skills' folder
	cwd, err := os.Getwd()
	if err == nil {
		allowedDirectories = append(allowedDirectories, cwd)
		log.Printf("FileSystemTool: Added current working directory to allowed paths: %s", cwd)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Warning: Could not get user home directory: %v. Cannot construct default allowed path.", err)
	} else {
		combinedPath := filepath.Join(homeDir, "teo_home")
		allowedDirectories = append(allowedDirectories, combinedPath)
		log.Printf("FileSystemTool: Default allowed directory set to %s", combinedPath)
	}
}

// TODO: Make allowedDirectories configurable (e.g., via environment variable, config file)

type FileSystemTool struct{}

func NewFileSystemTool() *FileSystemTool {
	return &FileSystemTool{}
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
}

func isAllowed(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("error getting absolute path: %w", err)
	}

	for _, allowedDir := range allowedDirectories {
		absAllowedDir, err := filepath.Abs(allowedDir)
		if err != nil {
			// Log or handle error in resolving allowedDir
			continue
		}
		if strings.HasPrefix(absPath, absAllowedDir) {
			return absPath, nil
		}
	}
	return "", fmt.Errorf("path '%s' (resolved to '%s') is not within allowed directories", path, absPath)
}

func (f *FileSystemTool) CallTool(arguments string) string {
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
		return f.readFile(args.Path)
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
		return f.listDirectory(args.Path)
	case "directory_tree":
		return f.directoryTree(args.Path)
	case "move_file":
		return f.moveFile(args.OldPath, args.NewPath)
	case "search_files":
		// Assuming args.Path is the directory to search in and args.Pattern is the search pattern
		return f.searchFiles(args.Path, args.Pattern)
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

// Implement private methods for each file system operation here.
// Example for readFile:
func (f *FileSystemTool) readFile(path string) string {
	absPath, err := isAllowed(path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("Error reading file %s: %v", path, err)
	}
	return string(data)
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

func (f *FileSystemTool) directoryTree(basePath string) string {
	absBasePath, err := isAllowed(basePath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	var buildTree func(currentPath string) (DirEntry, error)
	buildTree = func(currentPath string) (DirEntry, error) {
		info, err := os.Stat(currentPath)
		if err != nil {
			return DirEntry{}, fmt.Errorf("error stating path %s: %w", currentPath, err)
		}

		entry := DirEntry{
			Name: filepath.Base(currentPath),
		}

		if info.IsDir() {
			entry.Type = "directory"
			entry.Children = []DirEntry{} // Initialize, even if empty

			files, err := os.ReadDir(currentPath)
			if err != nil {
				return DirEntry{}, fmt.Errorf("error reading directory %s: %w", currentPath, err)
			}

			for _, file := range files {
				childPath := filepath.Join(currentPath, file.Name())
				// Security check for child paths is implicitly handled by the initial basePath check
				// if traversal outside allowed directories is a concern at deeper levels,
				// an additional isAllowed check could be added here for childPath.
				// However, if basePath is allowed, all its children should be too unless symlinks point outside.
				// For simplicity and given the current isAllowed logic, we assume subdirectories are fine.
				childEntry, err := buildTree(childPath)
				if err != nil {
					// Decide how to handle errors for individual children, e.g., skip or return error
					// For now, let's skip problematic children but log the error
					fmt.Printf("Skipping child %s due to error: %v\n", childPath, err)
					continue
				}
				entry.Children = append(entry.Children, childEntry)
			}
		} else {
			entry.Type = "file"
			// Files do not have children, so Children remains nil (or empty if initialized)
		}
		return entry, nil
	}

	rootEntry, err := buildTree(absBasePath)
	if err != nil {
		return fmt.Sprintf("Error building directory tree for %s: %v", basePath, err)
	}

	// Correctly marshal the root entry which represents the initial basePath directory itself
	// The issue asks for the children of the basePath to be the primary list if basePath is a directory.
	// The current buildTree returns the basePath itself as the root DirEntry.
	// If the root is a directory, we should marshal its Children. If it's a file, marshal the entry itself.
	var dataToMarshal interface{}
	if rootEntry.Type == "directory" {
		// The spec implies the output is an array of entries *within* the directory,
		// or a single entry if path is a file.
		// Let's adjust to return the root entry itself for consistency with get_file_info.
		// The JSON structure {name, type, children} seems to describe the node itself.
		dataToMarshal = rootEntry
	} else {
		dataToMarshal = rootEntry // A file
	}

	jsonData, err := json.MarshalIndent(dataToMarshal, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error marshalling directory tree to JSON: %v", err)
	}
	return string(jsonData)
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
