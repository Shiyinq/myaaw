package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type NoteTool struct{}

func NewNotesTool() ToolsFactory {
	return &NoteTool{}
}

type NoteArguments struct {
	Action  string `json:"action"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (n *NoteTool) CallTool(arguments string) string {
	var args NoteArguments
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	switch strings.ToUpper(args.Action) {
	case "GET":
		return n.getNotes()
	case "GET_DETAIL":
		return n.getNoteDetail(args.Title)
	case "POST":
		return n.saveNote(args.Title, args.Content)
	case "PUT":
		return n.updateNote(args.Title, args.Content)
	case "DELETE":
		return n.deleteNote(args.Title)
	default:
		return "Invalid action specified. Please use GET, GET_DETAIL, POST, PUT, or DELETE."
	}
}

func (n *NoteTool) getNotes() string {
	files, err := os.ReadDir("./notes")
	if err != nil {
		return fmt.Sprintf("Error reading notes directory: %v", err)
	}

	var noteNames []string
	for _, file := range files {
		if !file.IsDir() {
			noteNames = append(noteNames, strings.TrimSuffix(file.Name(), ".txt"))
		}
	}

	return fmt.Sprintf("List of notes: %v", noteNames)
}

func (n *NoteTool) getNoteDetail(title string) string {
	filePath := fmt.Sprintf("./notes/%s.txt", title)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Sprintf("Error reading note %s: %v", title, err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return fmt.Sprintf("Error reading content of note %s: %v", title, err)
	}

	return fmt.Sprintf("Title: '%s'\nCatatan:\n%s", title, content)
}

func (n *NoteTool) saveNote(title, content string) string {
	dirPath := "./notes"
	filePath := filepath.Join(dirPath, fmt.Sprintf("%s.txt", title))

	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		return fmt.Sprintf("Error creating directory: %v", err)
	}

	if _, err := os.Stat(filePath); err == nil {
		return fmt.Sprintf("Note with title '%s' already exists. Use PUT to update it.", title)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Sprintf("Error saving note: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return fmt.Sprintf("Error writing to file: %v", err)
	}

	return fmt.Sprintf("Note '%s' has been saved successfully.", title)
}

func (n *NoteTool) updateNote(title, content string) string {
	filePath := fmt.Sprintf("./notes/%s.txt", title)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Sprintf("Note '%s' does not exist. Use POST to create it.", title)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Sprintf("Error opening note for update: %v", err)
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		return fmt.Sprintf("Error writing to file: %v", err)
	}

	return fmt.Sprintf("Note '%s' has been updated successfully.", title)
}

func (n *NoteTool) deleteNote(title string) string {
	filePath := fmt.Sprintf("./notes/%s.txt", title)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Sprintf("Note '%s' does not exist.", title)
	}

	err := os.Remove(filePath)
	if err != nil {
		return fmt.Sprintf("Error deleting note: %v", err)
	}

	return fmt.Sprintf("Note '%s' has been deleted successfully.", title)
}
