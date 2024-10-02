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

func CommandMe(res *model.User) string {
	var me strings.Builder
	me.WriteString("ℹ️*About Me*\n")
	me.WriteString(fmt.Sprintf("*ID:* %d\n", res.UserId))
	me.WriteString(fmt.Sprintf("*Name:* %s\n", res.Name))
	me.WriteString("\n\n🛠️*Config*\n")
	me.WriteString(fmt.Sprintf("*System:* %s\n", res.System))
	me.WriteString(fmt.Sprintf("*Model:* %s\n", res.Model))
	me.WriteString(fmt.Sprintf("*History:* %d\n", len(res.Messages)))

	return me.String()
}
