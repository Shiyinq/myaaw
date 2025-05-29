package scraping

import (
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
)

type ScrapingTool struct{}

func NewScrapingTool() *ScrapingTool {
	return &ScrapingTool{}
}

type ScrapingArguments struct {
	Url string `json:"url"`
}

func (s *ScrapingTool) CallTool(arguments string) string {
	var args ScrapingArguments
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	apiUrl := fmt.Sprintf("https://r.jina.ai/%s", args.Url)

	client := resty.New()

	resp, err := client.R().Get(apiUrl)
	if err != nil {
		return fmt.Sprintf("Error making request: %v", err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Sprintf("Error response from API: %s", resp.Status())
	}

	return resp.String()
}
