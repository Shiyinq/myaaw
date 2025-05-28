package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

const (
	tavilyAPIURL = "https://api.tavily.com"
)

type TavilyTool struct{}

func NewTavilyTool() *TavilyTool {
	return &TavilyTool{}
}

type TavilySearchRequest struct {
	Query                    string   `json:"query"`
	Topic                    string   `json:"topic,omitempty"`
	SearchDepth              string   `json:"search_depth,omitempty"`
	ChunksPerSource          int      `json:"chunks_per_source,omitempty"`
	MaxResults               int      `json:"max_results,omitempty"`
	TimeRange                string   `json:"time_range,omitempty"`
	Days                     int      `json:"days,omitempty"`
	IncludeAnswer            bool     `json:"include_answer,omitempty"`
	IncludeRawContent        bool     `json:"include_raw_content,omitempty"`
	IncludeImages            bool     `json:"include_images,omitempty"`
	IncludeImageDescriptions bool     `json:"include_image_descriptions,omitempty"`
	IncludeDomains           []string `json:"include_domains,omitempty"`
	ExcludeDomains           []string `json:"exclude_domains,omitempty"`
}

type TavilyExtractRequest struct {
	URLs          string `json:"urls"`
	IncludeImages bool   `json:"include_images,omitempty"`
	ExtractDepth  string `json:"extract_depth,omitempty"`
}

type TavilyToolInput struct {
	Action      string                `json:"action"`
	SearchArgs  *TavilySearchRequest  `json:"search_args,omitempty"`
	ExtractArgs *TavilyExtractRequest `json:"extract_args,omitempty"`
}

func (t *TavilyTool) CallTool(arguments string) string {
	_ = godotenv.Load()

	var input TavilyToolInput
	err := json.Unmarshal([]byte(arguments), &input)
	if err != nil {
		return fmt.Sprintf("Error unmarshalling arguments: %v", err)
	}

	apiKey := os.Getenv("TAVILY_API_KEY")
	if apiKey == "" {
		return "Error: TAVILY_API_KEY environment variable not set in .env file or system environment."
	}

	var endpoint string
	var reqBody []byte

	switch input.Action {
	case "search":
		if input.SearchArgs == nil {
			return "Error: search_args are required for search action."
		}
		endpoint = "/search"
		reqBody, err = json.Marshal(input.SearchArgs)
		if err != nil {
			return fmt.Sprintf("Error marshalling search request body: %v", err)
		}
	case "extract":
		if input.ExtractArgs == nil {
			return "Error: extract_args are required for extract action."
		}
		endpoint = "/extract"
		reqBody, err = json.Marshal(input.ExtractArgs)
		if err != nil {
			return fmt.Sprintf("Error marshalling extract request body: %v", err)
		}
	default:
		return fmt.Sprintf("Error: Invalid action '%s'. Must be 'search' or 'extract'.", input.Action)
	}

	client := &http.Client{}
	req, err := http.NewRequest("POST", tavilyAPIURL+endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Sprintf("Error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Error making request to Tavily API: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("Error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Error: Tavily API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Printf("Successfully called Tavily API endpoint '%s'. Response: %s", endpoint, string(respBody))
	return string(respBody)
}
