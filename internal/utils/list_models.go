package utils

import (
	"fmt"
	"strings"
	"teo/internal/services/bot/model"
)

func ListModels(response model.OllamaTagsResponse) string {
	var result strings.Builder
	result.WriteString("Available Models\n\n")
	for i, model := range response.Models {
		result.WriteString(fmt.Sprintf("%d - %s\n", i, model.Name))
	}
	result.WriteString("\n\nUsage: /models <number>\nexample: /models 0")
	return result.String()
}
