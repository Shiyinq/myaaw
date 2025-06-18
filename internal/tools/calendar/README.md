# Calendar Management Tool

A comprehensive tool for managing schedules and events with full CRUD operations, search capabilities, and tag-based organization.

## Overview

The Calendar Tool provides a complete scheduling solution for creating, managing, and searching calendar events. It supports event scheduling, date-based searches, title searches, and tag-based organization with persistent JSON storage.

## Features

- **Event Management**: Create, update, delete, and retrieve events
- **Date Range Search**: Find events within specific date ranges
- **Title Search**: Search events by title with case-insensitive matching
- **Tag-based Organization**: Organize events with multiple tags
- **Time Management**: Start and end time support for events
- **JSON Storage**: Persistent data storage in JSON format
- **Flexible Search**: Multiple search criteria and methods

## Data Structure

### Schedule Object

```json
{
  "id": "unique_event_id",
  "title": "Event Title",
  "description": "Event description",
  "start_time": "2024-01-01T10:00:00Z",
  "end_time": "2024-01-01T11:00:00Z",
  "tags": ["meeting", "work", "important"]
}
```

## Usage

### Available Actions

| Action | Description | Required Parameters |
|--------|-------------|-------------------|
| `add_schedule` | Add new event | `schedule` object |
| `update_schedule` | Update existing event | `id`, `schedule` object |
| `delete_schedule` | Delete event | `id` |
| `search_by_date` | Search events by date range | `start_date`, `end_date` |
| `search_by_title` | Search events by title | `title` |
| `search_by_tags` | Search events by tags | `tags` array |

### Parameters

| Parameter | Type | Required | Description | Format |
|-----------|------|----------|-------------|--------|
| `action` | string | Yes | Action to perform | See table above |
| `id` | string | Conditional | Event ID | Any string |
| `schedule` | object | Conditional | Event data | See Schedule Object |
| `start_date` | string | Conditional | Start date for search | YYYY-MM-DD |
| `end_date` | string | Conditional | End date for search | YYYY-MM-DD |
| `title` | string | Conditional | Title to search for | Any string |
| `tags` | array | Conditional | Tags to search for | Array of strings |

### Schedule Object Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `title` | string | Yes | Event title |
| `description` | string | No | Event description |
| `start_time` | string | Yes | Start time (ISO 8601) |
| `end_time` | string | Yes | End time (ISO 8601) |
| `tags` | array | No | Array of tag strings |

## Example Usage

### Add New Event

```json
{
  "action": "add_schedule",
  "schedule": {
    "title": "Team Meeting",
    "description": "Weekly team sync meeting",
    "start_time": "2024-01-01T10:00:00Z",
    "end_time": "2024-01-01T11:00:00Z",
    "tags": ["meeting", "work", "weekly"]
  }
}
```

### Search by Date Range

```json
{
  "action": "search_by_date",
  "start_date": "2024-01-01",
  "end_date": "2024-01-31"
}
```

### Search by Title

```json
{
  "action": "search_by_title",
  "title": "meeting"
}
```

### Search by Tags

```json
{
  "action": "search_by_tags",
  "tags": ["work", "important"]
}
```

### Update Event

```json
{
  "action": "update_schedule",
  "id": "event_id_123",
  "schedule": {
    "title": "Updated Team Meeting",
    "description": "Updated description",
    "start_time": "2024-01-01T10:00:00Z",
    "end_time": "2024-01-01T11:30:00Z",
    "tags": ["meeting", "work", "updated"]
  }
}
```

## Implementation Details

### Storage Location

Data is stored in: `data/calendar/calendar.json`

### Key Functions

- `NewCalendarTool()` - Creates new calendar tool instance
- `CallTool(arguments string)` - Main function that processes operations
- `NewCalendarManager()` - Creates calendar manager with data persistence
- `AddSchedule(schedule Schedule)` - Adds new event
- `UpdateSchedule(id string, schedule Schedule)` - Updates existing event
- `DeleteSchedule(id string)` - Deletes event
- `SearchByDateRange(start, end time.Time)` - Searches by date range
- `SearchByTitle(title string)` - Searches by title
- `SearchByTags(tags []string)` - Searches by tags

### Data Processing

1. **Input Validation**: Validates required parameters and data types
2. **Date Parsing**: Converts date strings to time.Time objects
3. **Data Persistence**: JSON file operations for storage
4. **Search Processing**: Multiple search algorithms
5. **Error Handling**: Comprehensive error handling for all operations

## Search Features

### Date Range Search

- **Inclusive Range**: Both start and end dates are included
- **Time-aware**: Considers start and end times of events
- **Overlap Detection**: Finds events that overlap with the date range
- **Format**: YYYY-MM-DD (ISO date format)

### Title Search

- **Case-insensitive**: Searches work regardless of case
- **Partial Matching**: Finds events containing search terms
- **Multiple Results**: Returns all matching events
- **Fuzzy Matching**: Flexible title matching

### Tag Search

- **Multiple Tags**: Search by one or more tags
- **OR Logic**: Events matching any of the specified tags
- **Case-sensitive**: Tag matching is case-sensitive
- **Exact Matching**: Requires exact tag matches

## Error Handling

- Missing required parameters
- Invalid date formats
- Non-existent event IDs
- File system errors
- JSON parsing errors
- Duplicate event IDs

## Response Formats

### Success Response

```json
{
  "message": "Event added successfully",
  "id": "generated_event_id"
}
```

### Search Results

```json
[
  {
    "id": "event_id_1",
    "title": "Team Meeting",
    "description": "Weekly team sync",
    "start_time": "2024-01-01T10:00:00Z",
    "end_time": "2024-01-01T11:00:00Z",
    "tags": ["meeting", "work"]
  }
]
```

### Error Response

```json
{
  "error": "Event with ID 'event_id' not found"
}
```

## Security Considerations

- Local file storage only
- Input validation for all parameters
- Date format validation
- Error message sanitization
- No external API calls

## Limitations

- Local storage only (no cloud sync)
- No recurring event support
- No calendar view generation
- No event reminders
- No multi-user support
- No event sharing capabilities
- No calendar export/import

## Best Practices

- Use descriptive event titles for easy searching
- Add relevant tags for better organization
- Use consistent date formats (ISO 8601)
- Regular backups of calendar data
- Validate event times (end > start)
- Use meaningful event descriptions

## Use Cases

- **Personal Scheduling**: Manage personal appointments and events
- **Team Coordination**: Schedule team meetings and events
- **Project Management**: Track project milestones and deadlines
- **Event Planning**: Organize events with tags and descriptions
- **Time Tracking**: Monitor time spent on different activities

## Performance Considerations

- Efficient JSON file operations
- Fast search algorithms
- Minimal memory usage
- Optimized date range queries
- Quick tag-based filtering

## Examples

### Complete Workflow

```json
// 1. Add an event
{
  "action": "add_schedule",
  "schedule": {
    "title": "Project Review",
    "description": "Monthly project review meeting",
    "start_time": "2024-01-15T14:00:00Z",
    "end_time": "2024-01-15T15:00:00Z",
    "tags": ["meeting", "project", "review"]
  }
}

// 2. Search for project-related events
{
  "action": "search_by_tags",
  "tags": ["project"]
}

// 3. Search for events in January
{
  "action": "search_by_date",
  "start_date": "2024-01-01",
  "end_date": "2024-01-31"
}
```  
