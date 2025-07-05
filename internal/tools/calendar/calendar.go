package calendar

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Schedule struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Tags        []string  `json:"tags"`
}

type CalendarManager struct {
	dataPath  string
	schedules []Schedule
}

func NewCalendarManager() (*CalendarManager, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		log.Printf("Error getting working directory: %v\n", err)
		return nil, err
	}

	dataDir := filepath.Join(workingDir, "data", "calendar")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Printf("Error creating calendar directory: %v\n", err)
		return nil, err
	}

	dataPath := filepath.Join(dataDir, "calendar.json")
	manager := &CalendarManager{
		dataPath:  dataPath,
		schedules: []Schedule{},
	}

	if err := manager.loadSchedules(); err != nil {
		log.Printf("Warning: error loading schedules: %v", err)
	}

	return manager, nil
}

func (cm *CalendarManager) loadSchedules() error {
	if _, err := os.Stat(cm.dataPath); err == nil {
		data, err := os.ReadFile(cm.dataPath)
		if err != nil {
			return fmt.Errorf("error reading data file: %v", err)
		}

		if len(data) == 0 {
			return nil
		}

		if err := json.Unmarshal(data, &cm.schedules); err != nil {
			log.Printf("Warning: error parsing data file: %v, returning empty data", err)
			return nil
		}
	}
	return nil
}

func (cm *CalendarManager) saveSchedules() error {
	if cm.schedules == nil {
		cm.schedules = make([]Schedule, 0)
	}

	data, err := json.MarshalIndent(cm.schedules, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling data: %v", err)
	}

	dir := filepath.Dir(cm.dataPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	if err := os.WriteFile(cm.dataPath, data, 0644); err != nil {
		return fmt.Errorf("error writing data file: %v", err)
	}

	return nil
}

func (cm *CalendarManager) AddSchedule(schedule Schedule) error {
	cm.schedules = append(cm.schedules, schedule)
	return cm.saveSchedules()
}

func (cm *CalendarManager) UpdateSchedule(id string, userID string, updatedSchedule Schedule) error {
	for i, s := range cm.schedules {
		if s.ID == id && s.UserID == userID {
			cm.schedules[i] = updatedSchedule
			return cm.saveSchedules()
		}
	}
	return fmt.Errorf("schedule with ID %s not found", id)
}

func (cm *CalendarManager) DeleteSchedule(id string, userID string) error {
	for i, s := range cm.schedules {
		if s.ID == id && s.UserID == userID {
			cm.schedules = append(cm.schedules[:i], cm.schedules[i+1:]...)
			return cm.saveSchedules()
		}
	}
	return fmt.Errorf("schedule with ID %s not found", id)
}

func (cm *CalendarManager) SearchByDateRange(userID string, start, end time.Time) []Schedule {
	var results []Schedule
	for _, s := range cm.schedules {
		if s.UserID == userID && (s.StartTime.Equal(start) || s.StartTime.After(start)) && (s.EndTime.Equal(end) || s.EndTime.Before(end)) {
			results = append(results, s)
		}
	}
	return results
}

func (cm *CalendarManager) SearchByTitle(userID string, title string) []Schedule {
	var results []Schedule
	searchTitle := strings.ToLower(title)
	for _, s := range cm.schedules {
		if s.UserID == userID && strings.Contains(strings.ToLower(s.Title), searchTitle) {
			results = append(results, s)
		}
	}
	return results
}

func (cm *CalendarManager) SearchByTags(userID string, tags []string) []Schedule {
	var results []Schedule
	for _, s := range cm.schedules {
		if s.UserID != userID {
			continue
		}
		for _, tag := range tags {
			for _, scheduleTag := range s.Tags {
				if tag == scheduleTag {
					results = append(results, s)
					break
				}
			}
		}
	}
	return results
}

type CalendarTool struct {
	manager *CalendarManager
}

func NewCalendarTool() *CalendarTool {
	manager, err := NewCalendarManager()
	if err != nil {
		log.Printf("Error creating calendar manager: %v", err)
		return nil
	}
	return &CalendarTool{manager: manager}
}

func (ct *CalendarTool) CallTool(arguments string) string {
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	action, ok := params["action"].(string)
	if !ok {
		return "Error: action not found"
	}

	switch action {
	case "add_schedule":
		return ct.handleAddSchedule(params)
	case "update_schedule":
		return ct.handleUpdateSchedule(params)
	case "delete_schedule":
		return ct.handleDeleteSchedule(params)
	case "search_by_date":
		return ct.handleSearchByDate(params)
	case "search_by_title":
		return ct.handleSearchByTitle(params)
	case "search_by_tags":
		return ct.handleSearchByTags(params)
	default:
		return fmt.Sprintf("Error: invalid action: %s", action)
	}
}

func (ct *CalendarTool) handleAddSchedule(params map[string]interface{}) string {
	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	scheduleData, ok := params["schedule"].(map[string]interface{})
	if !ok {
		return "Error: invalid schedule data"
	}

	title, ok := scheduleData["title"].(string)
	if !ok {
		return "Error: title is required"
	}

	description, ok := scheduleData["description"].(string)
	if !ok {
		return "Error: description is required"
	}

	startTimeStr, ok := scheduleData["start_time"].(string)
	if !ok {
		return "Error: start_time is required"
	}
	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		return fmt.Sprintf("Error: invalid start_time format: %v", err)
	}

	endTimeStr, ok := scheduleData["end_time"].(string)
	if !ok {
		return "Error: end_time is required"
	}
	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		return fmt.Sprintf("Error: invalid end_time format: %v", err)
	}

	tagsInterface, ok := scheduleData["tags"].([]interface{})
	if !ok {
		return "Error: tags is required"
	}
	tags := make([]string, len(tagsInterface))
	for i, tag := range tagsInterface {
		tags[i], ok = tag.(string)
		if !ok {
			return "Error: invalid tag format"
		}
	}

	schedule := Schedule{
		ID:          fmt.Sprintf("%d", time.Now().UnixNano()),
		UserID:      userID,
		Title:       title,
		Description: description,
		StartTime:   startTime,
		EndTime:     endTime,
		Tags:        tags,
	}

	if err := ct.manager.AddSchedule(schedule); err != nil {
		return fmt.Sprintf("Error adding schedule: %v", err)
	}

	return "Schedule added successfully"
}

func (ct *CalendarTool) handleUpdateSchedule(params map[string]interface{}) string {
	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	scheduleData, ok := params["schedule"].(map[string]interface{})
	if !ok {
		return "Error: invalid schedule data"
	}

	id, ok := scheduleData["id"].(string)
	if !ok {
		return "Error: id is required"
	}

	title, ok := scheduleData["title"].(string)
	if !ok {
		return "Error: title is required"
	}

	description, ok := scheduleData["description"].(string)
	if !ok {
		return "Error: description is required"
	}

	startTimeStr, ok := scheduleData["start_time"].(string)
	if !ok {
		return "Error: start_time is required"
	}
	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		return fmt.Sprintf("Error: invalid start_time format: %v", err)
	}

	endTimeStr, ok := scheduleData["end_time"].(string)
	if !ok {
		return "Error: end_time is required"
	}
	endTime, err := time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		return fmt.Sprintf("Error: invalid end_time format: %v", err)
	}

	tagsInterface, ok := scheduleData["tags"].([]interface{})
	if !ok {
		return "Error: tags is required"
	}
	tags := make([]string, len(tagsInterface))
	for i, tag := range tagsInterface {
		tags[i], ok = tag.(string)
		if !ok {
			return "Error: invalid tag format"
		}
	}

	schedule := Schedule{
		ID:          id,
		UserID:      userID,
		Title:       title,
		Description: description,
		StartTime:   startTime,
		EndTime:     endTime,
		Tags:        tags,
	}

	if err := ct.manager.UpdateSchedule(id, userID, schedule); err != nil {
		return fmt.Sprintf("Error updating schedule: %v", err)
	}

	return "Schedule updated successfully"
}

func (ct *CalendarTool) handleDeleteSchedule(params map[string]interface{}) string {
	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	id, ok := params["schedule_id"].(string)
	if !ok {
		return "Error: schedule_id is required"
	}

	if err := ct.manager.DeleteSchedule(id, userID); err != nil {
		return fmt.Sprintf("Error deleting schedule: %v", err)
	}

	return "Schedule deleted successfully"
}

func (ct *CalendarTool) handleSearchByDate(params map[string]interface{}) string {
	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	dateRange, ok := params["date_range"].(map[string]interface{})
	if !ok {
		return "Error: date_range is required"
	}

	startStr, ok := dateRange["start"].(string)
	if !ok {
		return "Error: start date is required"
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return fmt.Sprintf("Error: invalid start date format: %v", err)
	}

	endStr, ok := dateRange["end"].(string)
	if !ok {
		return "Error: end date is required"
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return fmt.Sprintf("Error: invalid end date format: %v", err)
	}

	schedules := ct.manager.SearchByDateRange(userID, start, end)
	result, err := json.Marshal(schedules)
	if err != nil {
		return fmt.Sprintf("Error marshaling results: %v", err)
	}

	return string(result)
}

func (ct *CalendarTool) handleSearchByTitle(params map[string]interface{}) string {
	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	title, ok := params["title"].(string)
	if !ok {
		return "Error: title is required"
	}

	schedules := ct.manager.SearchByTitle(userID, title)
	result, err := json.Marshal(schedules)
	if err != nil {
		return fmt.Sprintf("Error marshaling results: %v", err)
	}

	return string(result)
}

func (ct *CalendarTool) handleSearchByTags(params map[string]interface{}) string {
	userID, ok := params["user_id"].(string)
	if !ok || userID == "" {
		return "Error: user_id is required"
	}

	tagsInterface, ok := params["tags"].([]interface{})
	if !ok {
		return "Error: tags is required"
	}

	tags := make([]string, len(tagsInterface))
	for i, tag := range tagsInterface {
		tags[i], ok = tag.(string)
		if !ok {
			return "Error: invalid tag format"
		}
	}

	schedules := ct.manager.SearchByTags(userID, tags)
	result, err := json.Marshal(schedules)
	if err != nil {
		return fmt.Sprintf("Error marshaling results: %v", err)
	}

	return string(result)
}
