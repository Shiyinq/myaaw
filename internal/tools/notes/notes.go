package notes

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type NoteTool struct {
	dataPath string
}

func NewNotesTool() *NoteTool {
	workingDir, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting working directory: %v\n", err)
		return nil
	}

	dataDir := filepath.Join(workingDir, "data", "notes")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Printf("Error creating notes directory: %v\n", err)
		return nil
	}

	return &NoteTool{
		dataPath: dataDir,
	}
}

type NoteArguments struct {
	Action    string `json:"action"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Search    string `json:"search"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type Note struct {
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (n *NoteTool) CallTool(arguments string) string {
	var args NoteArguments
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	if err := n.validateInput(args); err != nil {
		return fmt.Sprintf("Validation error: %v", err)
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
	case "SEARCH":
		return n.searchNotes(args.Search)
	case "GET_BY_DATE":
		return n.getNotesByDate(args.StartDate, args.EndDate)
	default:
		return "Invalid action specified. Please use GET, GET_DETAIL, POST, PUT, DELETE, SEARCH, or GET_BY_DATE."
	}
}

func (n *NoteTool) validateInput(args NoteArguments) error {
	if args.Action == "" {
		return fmt.Errorf("action is required")
	}

	if args.Action == "POST" || args.Action == "PUT" {
		if args.Title == "" {
			return fmt.Errorf("title is required")
		}
		if args.Content == "" {
			return fmt.Errorf("content is required")
		}
	}

	return nil
}

func (n *NoteTool) getNotes() string {
	files, err := os.ReadDir(n.dataPath)
	if err != nil {
		return fmt.Sprintf("Error reading notes directory: %v", err)
	}

	var notes []Note
	for _, file := range files {
		if !file.IsDir() {
			note, err := n.readNoteFile(file.Name())
			if err != nil {
				continue
			}
			notes = append(notes, note)
		}
	}

	jsonNotes, err := json.Marshal(notes)
	if err != nil {
		return fmt.Sprintf("Error marshaling notes: %v", err)
	}

	return string(jsonNotes)
}

func (n *NoteTool) readNoteFile(filename string) (Note, error) {
	filePath := filepath.Join(n.dataPath, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return Note{}, err
	}

	var note Note
	if err := json.Unmarshal(content, &note); err != nil {
		return Note{}, err
	}

	return note, nil
}

func (n *NoteTool) getNoteDetail(title string) string {
	note, err := n.readNoteFile(fmt.Sprintf("%s.json", title))
	if err != nil {
		return fmt.Sprintf("Error reading note %s: %v", title, err)
	}

	jsonNote, err := json.Marshal(note)
	if err != nil {
		return fmt.Sprintf("Error marshaling note: %v", err)
	}

	return string(jsonNote)
}

func (n *NoteTool) saveNote(title, content string) string {
	filePath := filepath.Join(n.dataPath, fmt.Sprintf("%s.json", title))

	if _, err := os.Stat(filePath); err == nil {
		return fmt.Sprintf("Note with title '%s' already exists. Use PUT to update it.", title)
	}

	note := Note{
		Title:     title,
		Content:   content,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	jsonNote, err := json.Marshal(note)
	if err != nil {
		return fmt.Sprintf("Error marshaling note: %v", err)
	}

	if err := os.WriteFile(filePath, jsonNote, 0644); err != nil {
		return fmt.Sprintf("Error saving note: %v", err)
	}

	return fmt.Sprintf("Note '%s' has been saved successfully.", title)
}

func (n *NoteTool) updateNote(title, content string) string {
	filePath := filepath.Join(n.dataPath, fmt.Sprintf("%s.json", title))

	note, err := n.readNoteFile(fmt.Sprintf("%s.json", title))
	if err != nil {
		return fmt.Sprintf("Note '%s' does not exist. Use POST to create it.", title)
	}

	note.Content = content
	note.UpdatedAt = time.Now()

	jsonNote, err := json.Marshal(note)
	if err != nil {
		return fmt.Sprintf("Error marshaling note: %v", err)
	}

	if err := os.WriteFile(filePath, jsonNote, 0644); err != nil {
		return fmt.Sprintf("Error updating note: %v", err)
	}

	return fmt.Sprintf("Note '%s' has been updated successfully.", title)
}

func (n *NoteTool) deleteNote(title string) string {
	filePath := filepath.Join(n.dataPath, fmt.Sprintf("%s.json", title))

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Sprintf("Note '%s' does not exist.", title)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Sprintf("Error deleting note: %v", err)
	}

	return fmt.Sprintf("Note '%s' has been deleted successfully.", title)
}

func (n *NoteTool) searchNotes(query string) string {
	files, err := os.ReadDir(n.dataPath)
	if err != nil {
		return fmt.Sprintf("Error reading notes directory: %v", err)
	}

	var results []Note
	for _, file := range files {
		if !file.IsDir() {
			note, err := n.readNoteFile(file.Name())
			if err != nil {
				continue
			}
			if strings.Contains(strings.ToLower(note.Title), strings.ToLower(query)) ||
				strings.Contains(strings.ToLower(note.Content), strings.ToLower(query)) {
				results = append(results, note)
			}
		}
	}

	jsonResults, err := json.Marshal(results)
	if err != nil {
		return fmt.Sprintf("Error marshaling search results: %v", err)
	}

	return string(jsonResults)
}

func (n *NoteTool) getNotesByDate(startDate, endDate string) string {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return fmt.Sprintf("Invalid start date format. Use YYYY-MM-DD: %v", err)
	}

	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return fmt.Sprintf("Invalid end date format. Use YYYY-MM-DD: %v", err)
	}

	files, err := os.ReadDir(n.dataPath)
	if err != nil {
		return fmt.Sprintf("Error reading notes directory: %v", err)
	}

	var results []Note
	for _, file := range files {
		if !file.IsDir() {
			note, err := n.readNoteFile(file.Name())
			if err != nil {
				continue
			}
			if note.CreatedAt.After(start) && note.CreatedAt.Before(end.Add(24*time.Hour)) {
				results = append(results, note)
			}
		}
	}

	jsonResults, err := json.Marshal(results)
	if err != nil {
		return fmt.Sprintf("Error marshaling date results: %v", err)
	}

	return string(jsonResults)
}
