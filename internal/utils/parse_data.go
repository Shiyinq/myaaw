package utils

import (
	"fmt"
	"strings"
	"teo/internal/services/bot/model"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func ListModels(user model.User, provider string, models []string) string {
	var result strings.Builder
	provider = cases.Title(language.Und).String(provider)
	result.WriteString(fmt.Sprintf("🧠 %s Available Models\n\n", provider))
	for i := range models {
		status := ""
		if models[i] == user.Model {
			status = " ✅*Actived*"
		}
		result.WriteString(fmt.Sprintf("%d - %s%s\n", i, models[i], status))
	}
	result.WriteString("\n\nUsage: /models <number>\nExample: /models 0")
	return result.String()
}

func CommandMe(res *model.User) string {
	var me strings.Builder
	me.WriteString("ℹ️ *About Me*\n")
	me.WriteString(fmt.Sprintf("*ID:* %d\n", res.UserId))
	me.WriteString(fmt.Sprintf("*Name:* %s\n", res.Name))
	me.WriteString("\n\n🛠️ *Config*\n")
	me.WriteString(fmt.Sprintf("*System:* %s\n", res.System))
	me.WriteString(fmt.Sprintf("*Model:* %s\n", res.Model))

	return me.String()
}
