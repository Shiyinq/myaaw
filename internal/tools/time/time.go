package time

import (
	"encoding/json"
	"fmt"
	"time"
)

type TimeTool struct{}

type TimeRequest struct {
	Timezone string `json:"timezone"`
}

type TimeResponse struct {
	CurrentTime time.Time `json:"current_time"`
	Timezone    string    `json:"timezone"`
	Year        int       `json:"year"`
	Month       int       `json:"month"`
	Day         int       `json:"day"`
	Hour        int       `json:"hour"`
	Minute      int       `json:"minute"`
	Second      int       `json:"second"`
	Weekday     string    `json:"weekday"`
}

func NewTimeTool() *TimeTool {
	return &TimeTool{}
}

func (t *TimeTool) CallTool(arguments string) string {
	var req TimeRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return fmt.Sprintf(`{"error": "Invalid arguments format: %v"}`, err)
	}

	var loc *time.Location
	var err error

	if req.Timezone == "" {
		loc = time.Local
	} else {
		loc, err = time.LoadLocation(req.Timezone)
		if err != nil {
			return fmt.Sprintf(`{"error": "Invalid timezone: %v"}`, err)
		}
	}

	now := time.Now().In(loc)
	response := TimeResponse{
		CurrentTime: now,
		Timezone:    loc.String(),
		Year:        now.Year(),
		Month:       int(now.Month()),
		Day:         now.Day(),
		Hour:        now.Hour(),
		Minute:      now.Minute(),
		Second:      now.Second(),
		Weekday:     now.Weekday().String(),
	}

	result, err := json.Marshal(response)
	if err != nil {
		return fmt.Sprintf(`{"error": "Failed to marshal response: %v"}`, err)
	}

	return string(result)
}
