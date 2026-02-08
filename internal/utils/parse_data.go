package utils

import (
	"fmt"
	"regexp"
	"strings"
	"myaaw/internal/services/bot/model"

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

func EscapeMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"`", "\\`",
	)
	return replacer.Replace(text)
}

func CommandMe(res *model.User) string {
	var me strings.Builder
	me.WriteString("ℹ️ *About Me*\n")
	me.WriteString(fmt.Sprintf("*ID:* %d\n", res.UserId))
	me.WriteString(fmt.Sprintf("*Name:* %s\n", EscapeMarkdown(res.Name)))
	me.WriteString("\n\n🛠️ *Config*\n")
	me.WriteString(fmt.Sprintf("*System:* %s\n", EscapeMarkdown(res.System)))
	me.WriteString(fmt.Sprintf("*Model:* %s\n", EscapeMarkdown(res.Model)))

	return me.String()
}

func ParseTelegramMarkdown(text string) string {
	// 1. Remove image markers ![alt](url) -> [alt](url)
	text = strings.ReplaceAll(text, "![", "[")

	// Placeholders
	const (
		markerBold   = "§BOLD§"
		markerItalic = "§ITALIC§"
		markerCode   = "§CODE§"
		markerLink   = "§LINK§"
		markerPre    = "§PRE§"
	)

	// 2a. Protect Preformatted Blocks (```...```)
	var preBlocks []string
	// Match triple backticks, optional language, content (including newlines), triple backticks
	rePre := regexp.MustCompile("(?s)```.*?```")
	text = rePre.ReplaceAllStringFunc(text, func(match string) string {
		preBlocks = append(preBlocks, match)
		return markerPre
	})

	// 2b. Protect Inline Code Blocks (`...`)
	var codeBlocks []string
	reCode := regexp.MustCompile("`[^`]+`")
	text = reCode.ReplaceAllStringFunc(text, func(match string) string {
		codeBlocks = append(codeBlocks, match)
		return markerCode
	})

	// 3. Protect Links [text](url)
	var links []string
	reLink := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	text = reLink.ReplaceAllStringFunc(text, func(match string) string {
		links = append(links, match)
		return markerLink
	})

	// 3b. Convert Headers (### Title -> **Title**)
	// Handle headers before other conversions so they become bold
	reHeader := regexp.MustCompile(`(?m)^[ \t]*#{1,6}[ \t]+(.*?)[ \t]*$`)
	text = reHeader.ReplaceAllString(text, "**$1**")

	// 4. Convert Markers
	// Bold ** -> *
	text = strings.ReplaceAll(text, "**", markerBold)
	// Italic __ -> _
	text = strings.ReplaceAll(text, "__", markerItalic)

	// Protect Bullets (* at start of line followed by space)
	const markerBullet = "§BULLET§"
	reBullet := regexp.MustCompile(`(^|\n)(\s*)\*\s`)
	text = reBullet.ReplaceAllString(text, "$1$2"+markerBullet)

	// Italic * -> _ (Handle *italic* syntax)
	// Match * followed by non-space, then content, then non-space, then *
	reAsteriskItalic := regexp.MustCompile(`\*([^\s*].*?[^\s*]|[^\s*])\*`)
	text = reAsteriskItalic.ReplaceAllString(text, markerItalic+"$1"+markerItalic)

	// 5. Escape Special Chars
	// Escape literal *
	text = strings.ReplaceAll(text, "*", "\\*")
	// Escape literal _
	text = strings.ReplaceAll(text, "_", "\\_")
	// Escape literal [
	text = strings.ReplaceAll(text, "[", "\\[")
	// Escape literal `
	text = strings.ReplaceAll(text, "`", "\\`")

	// 6. Restore Markers
	text = strings.ReplaceAll(text, markerBold, "*")
	text = strings.ReplaceAll(text, markerItalic, "_")
	text = strings.ReplaceAll(text, markerBullet, "* ")

	// 7. Restore Links
	for _, link := range links {
		text = strings.Replace(text, markerLink, link, 1)
	}

	// 8. Restore Code
	for _, code := range codeBlocks {
		text = strings.Replace(text, markerCode, code, 1)
	}

	// 9. Restore Preformatted Blocks
	for _, pre := range preBlocks {
		text = strings.Replace(text, markerPre, pre, 1)
	}

	return text
}
