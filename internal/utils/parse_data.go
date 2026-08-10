package utils

import (
	"fmt"
	"myaaw/internal/services/bot/model"
	"regexp"
	"strings"
)


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

func ParseTelegramHTML(text string) string {
	// 1. Remove image markers ![alt](url) -> [alt](url)
	text = strings.ReplaceAll(text, "![", "[")

	// 2. Protect Preformatted Blocks (```...```) -> <pre><code>...</code></pre>
	rePre := regexp.MustCompile("(?s)```(.*?)```")
	text = rePre.ReplaceAllString(text, "<pre>$1</pre>")

	// 3. Protect Inline Code Blocks (`...`) -> <code>...</code>
	reCode := regexp.MustCompile("`([^`]+)`")
	text = reCode.ReplaceAllString(text, "<code>$1</code>")

	// 4. Protect Links [text](url) -> <a href="url">text</a>
	reLink := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	text = reLink.ReplaceAllString(text, `<a href="$2">$1</a>`)

	// 5. Convert Headers (### Title -> <b>Title</b>)
	reHeader := regexp.MustCompile(`(?m)^[ \t]*#{1,6}[ \t]+(.*?)[ \t]*$`)
	text = reHeader.ReplaceAllString(text, "<b>$1</b>")

	// 6. Convert Bold **text** -> <b>text</b>
	reBold := regexp.MustCompile(`\*\*([^\*]+)\*\*`)
	text = reBold.ReplaceAllString(text, "<b>$1</b>")

	// 7. Convert Italic *text* -> <i>text</i>
	reItalicAss := regexp.MustCompile(`\*([^\*]+)\*`)
	text = reItalicAss.ReplaceAllString(text, "<i>$1</i>")

	// Convert Italic _text_ -> <i>text</i> (making sure no word boundaries get broken)
	reItalicUS := regexp.MustCompile(`\b_([^_]+)_\b`)
	text = reItalicUS.ReplaceAllString(text, "<i>$1</i>")

	// 8. Protect Bullets (* at start of line followed by space) -> we can just keep them as literal strings
	// HTML mode allows • or * just fine natively in text nodes

	return text
}

// StripMarkdown removes markdown entity characters so text can be streamed safely without
// unclosed formatting errors from the Telegram API.
func StripMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"**", "",
		"__", "",
		"*", "",
		"_", "",
		"`", "",
		"~", "",
		"[", "",
		"]", "",
		"(", "",
		")", "",
		"#", "",
		"!", "",
		">", "",
		".", "",
	)

	// Also strip markdown links [text](url) -> text
	reLink := regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	text = reLink.ReplaceAllString(text, "$1")

	return replacer.Replace(text)
}
