# Notes Management Tool

A comprehensive tool for managing personal notes with full CRUD operations, search capabilities, and date-based filtering.

## Overview

The Notes Tool provides a complete note-taking solution with support for creating, reading, updating, deleting, searching, and filtering notes by date. Notes are stored as JSON files in a local directory structure.

## Features

- **Full CRUD Operations**: Create, Read, Update, Delete notes
- **Search Functionality**: Search notes by title or content
- **Date Filtering**: Filter notes by creation date range
- **Metadata Tracking**: Automatic timestamps for creation and updates
- **JSON Storage**: Notes stored in structured JSON format
- **Case-insensitive Search**: Flexible search capabilities

## Data Structure

### Note Object

```json
{
  "title": "Note Title",
  "content": "Note content...",
  "created_at": "2024-01-01T10:00:00Z",
  "updated_at": "2024-01-01T10:00:00Z"
}
```

## Usage

### Available Actions

| Action | Description | Required Parameters |
|--------|-------------|-------------------|
| `GET` | Retrieve all notes | None |
| `GET_DETAIL` | Get specific note details | `title` |
| `POST` | Create new note | `title`, `content` |
| `PUT` | Update existing note | `title`, `content` |
| `DELETE` | Delete note | `title` |
| `SEARCH` | Search notes | `search` |
| `GET_BY_DATE` | Filter by date range | `start_date`, `end_date` |

### Parameters

| Parameter | Type | Required | Description | Format |
|-----------|------|----------|-------------|--------|
| `action` | string | Yes | Action to perform | See table above |
| `title` | string | Conditional | Note title | Any string |
| `content` | string | Conditional | Note content | Any string |
| `search` | string | Conditional | Search query | Any string |
| `start_date` | string | Conditional | Start date for filtering | YYYY-MM-DD |
| `end_date` | string | Conditional | End date for filtering | YYYY-MM-DD |

### Example Usage

#### Create a Note
```json
{
  "action": "POST",
  "title": "Meeting Notes",
  "content": "Discussion about project timeline and deliverables."
}
```

#### Search Notes
```json
{
  "action": "SEARCH",
  "search": "meeting"
}
```

#### Filter by Date Range
```json
{
  "action": "GET_BY_DATE",
  "start_date": "2024-01-01",
  "end_date": "2024-01-31"
}
```

## Implementation Details

### Storage Location

Notes are stored in: `data/notes/` directory relative to the working directory.

### File Naming Convention

Notes are saved as: `{title}.json`

### Key Functions

- `NewNotesTool()` - Creates a new notes tool instance
- `CallTool(arguments string)` - Main function that processes note operations
- `validateInput(args NoteArguments)` - Validates input parameters
- `getNotes()` - Retrieves all notes
- `saveNote(title, content string)` - Creates new note
- `updateNote(title, content string)` - Updates existing note
- `searchNotes(query string)` - Searches notes by content
- `getNotesByDate(startDate, endDate string)` - Filters notes by date

### Data Processing

1. **Input Validation**: Validates required parameters for each action
2. **File Operations**: Reads/writes JSON files for persistence
3. **Search Processing**: Case-insensitive text matching
4. **Date Parsing**: Converts date strings to time.Time objects
5. **Error Handling**: Comprehensive error handling for all operations

## Error Handling

- Missing required parameters
- File system errors
- JSON parsing errors
- Date format validation
- Duplicate note prevention
- Non-existent note operations

## Search Capabilities

- **Case-insensitive**: Searches work regardless of case
- **Content-based**: Searches both title and content
- **Partial matching**: Finds notes containing search terms
- **Multiple results**: Returns all matching notes

## Date Filtering

- **Format**: YYYY-MM-DD (ISO date format)
- **Range-based**: Filter notes created within date range
- **Inclusive**: Both start and end dates are included
- **Validation**: Ensures valid date formats

## Security Considerations

- Local file storage only
- Input validation for all parameters
- File path sanitization
- Error message sanitization

## Limitations

- Local storage only (no cloud sync)
- No rich text formatting
- No attachments support
- No tags or categories
- No version history
- File size limited by system

## Best Practices

- Use descriptive titles for easy searching
- Regular backups of the notes directory
- Avoid special characters in titles (used for filenames)
- Use consistent date formats for filtering 