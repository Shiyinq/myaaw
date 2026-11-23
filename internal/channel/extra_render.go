package channel

import (
	"encoding/json"
	"fmt"
	"strings"

	"myaaw/internal/provider"
)

// Shared decoded payload structs for extra_messages blocks, reused across
// Telegram, Discord, and CLI channel adapters.

type ExtraMediaItem struct {
	URL      string `json:"url,omitempty"`
	Path     string `json:"path,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Title    string `json:"title,omitempty"`
	Caption  string `json:"caption,omitempty"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type ExtraMediaData struct {
	Layout  string           `json:"layout,omitempty"`
	Caption string           `json:"caption,omitempty"`
	Items   []ExtraMediaItem `json:"items"`
}

type ExtraButton struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Type    string `json:"type"`
	Value   string `json:"value,omitempty"`
	URL     string `json:"url,omitempty"`
	Style   string `json:"style,omitempty"`
	Confirm string `json:"confirm,omitempty"`
}

type ExtraButtonsData struct {
	Prompt  string        `json:"prompt,omitempty"`
	Layout  string        `json:"layout,omitempty"`
	Buttons []ExtraButton `json:"buttons"`
}

type ExtraQuickReplyOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
}

type ExtraQuickRepliesData struct {
	Prompt  string                  `json:"prompt,omitempty"`
	Options []ExtraQuickReplyOption `json:"options"`
}

type ExtraCarouselCard struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Subtitle    string        `json:"subtitle,omitempty"`
	Description string        `json:"description,omitempty"`
	ImageURL    string        `json:"image_url,omitempty"`
	ImagePath   string        `json:"image_path,omitempty"`
	Badges      []string      `json:"badges,omitempty"`
	Buttons     []ExtraButton `json:"buttons,omitempty"`
}

type ExtraCarouselData struct {
	Cards []ExtraCarouselCard `json:"cards"`
}

type ExtraFormOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type ExtraFormField struct {
	Name        string            `json:"name"`
	Label       string            `json:"label"`
	Type        string            `json:"type"`
	Required    bool              `json:"required,omitempty"`
	Placeholder string            `json:"placeholder,omitempty"`
	Default     any               `json:"default,omitempty"`
	Help        string            `json:"help,omitempty"`
	Min         *float64          `json:"min,omitempty"`
	Max         *float64          `json:"max,omitempty"`
	Options     []ExtraFormOption `json:"options,omitempty"`
}

type ExtraFormData struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description,omitempty"`
	SubmitLabel string           `json:"submit_label,omitempty"`
	Fields      []ExtraFormField `json:"fields"`
}

type ExtraChecklistItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Checked     bool   `json:"checked,omitempty"`
}

type ExtraChecklistData struct {
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	SubmitLabel string               `json:"submit_label,omitempty"`
	Items       []ExtraChecklistItem `json:"items"`
}

type ExtraCalloutData struct {
	Variant string `json:"variant,omitempty"`
	Title   string `json:"title,omitempty"`
	Text    string `json:"text"`
}

type ExtraSummaryItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"`
}

type ExtraSummaryData struct {
	Title    string             `json:"title,omitempty"`
	Subtitle string             `json:"subtitle,omitempty"`
	Items    []ExtraSummaryItem `json:"items"`
}

type ExtraLinkPreviewData struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Domain      string `json:"domain,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
}

// FormatExtraMessagesPlain renders extra_messages as clean plain text for CLI
// and fallback surfaces.
func FormatExtraMessagesPlain(extras []provider.ExtraMessage) string {
	var parts []string
	for _, em := range extras {
		for _, block := range em.Blocks {
			if rendered := FormatExtraBlockFallback(block); rendered != "" {
				parts = append(parts, rendered)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// FormatExtraBlockFallback formats one ExtraBlock into readable Markdown/text
// for channels without a native widget for that block type.
func FormatExtraBlockFallback(block provider.ExtraBlock) string {
	switch block.Type {
	case "media":
		var data ExtraMediaData
		if json.Unmarshal(block.Data, &data) != nil || len(data.Items) == 0 {
			return ""
		}
		var lines []string
		for _, item := range data.Items {
			label := item.Title
			if label == "" {
				label = item.FileName
			}
			ref := item.URL
			if ref == "" {
				ref = item.Path
			}
			if label != "" && ref != "" {
				lines = append(lines, fmt.Sprintf("📎 %s — %s", label, ref))
			} else if ref != "" {
				lines = append(lines, "📎 "+ref)
			}
		}
		if data.Caption != "" {
			lines = append(lines, data.Caption)
		}
		return strings.Join(lines, "\n")

	case "buttons":
		var data ExtraButtonsData
		if json.Unmarshal(block.Data, &data) != nil || len(data.Buttons) == 0 {
			return ""
		}
		var lines []string
		if data.Prompt != "" {
			lines = append(lines, data.Prompt)
		}
		for i, btn := range data.Buttons {
			if btn.Type == "link" && btn.URL != "" {
				lines = append(lines, fmt.Sprintf("%d. %s: %s", i+1, btn.Label, btn.URL))
			} else {
				lines = append(lines, fmt.Sprintf("%d. [%s]", i+1, btn.Label))
			}
		}
		return strings.Join(lines, "\n")

	case "quick_replies":
		var data ExtraQuickRepliesData
		if json.Unmarshal(block.Data, &data) != nil || len(data.Options) == 0 {
			return ""
		}
		var lines []string
		if data.Prompt != "" {
			lines = append(lines, data.Prompt)
		}
		for i, opt := range data.Options {
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, opt.Label))
		}
		return strings.Join(lines, "\n")

	case "carousel":
		var data ExtraCarouselData
		if json.Unmarshal(block.Data, &data) != nil || len(data.Cards) == 0 {
			return ""
		}
		var cards []string
		for i, c := range data.Cards {
			var b strings.Builder
			fmt.Fprintf(&b, "%d. **%s**", i+1, c.Title)
			if c.Subtitle != "" {
				fmt.Fprintf(&b, " — %s", c.Subtitle)
			}
			if c.Description != "" {
				fmt.Fprintf(&b, "\n%s", c.Description)
			}
			if c.ImageURL != "" {
				fmt.Fprintf(&b, "\n%s", c.ImageURL)
			}
			for _, btn := range c.Buttons {
				if btn.Type == "link" && btn.URL != "" {
					fmt.Fprintf(&b, "\n• %s: %s", btn.Label, btn.URL)
				} else {
					fmt.Fprintf(&b, "\n• [%s]", btn.Label)
				}
			}
			cards = append(cards, b.String())
		}
		return strings.Join(cards, "\n\n")

	case "form":
		var data ExtraFormData
		if json.Unmarshal(block.Data, &data) != nil || len(data.Fields) == 0 {
			return ""
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("📋 **%s**", data.Title))
		if data.Description != "" {
			lines = append(lines, data.Description)
		}
		for i, f := range data.Fields {
			req := ""
			if f.Required {
				req = " *"
			}
			line := fmt.Sprintf("%d. %s%s", i+1, f.Label, req)
			if len(f.Options) > 0 {
				opts := make([]string, 0, len(f.Options))
				for _, o := range f.Options {
					opts = append(opts, o.Label)
				}
				line += " (" + strings.Join(opts, " / ") + ")"
			}
			lines = append(lines, line)
		}
		lines = append(lines, "_Reply with your answers to submit._")
		return strings.Join(lines, "\n")

	case "checklist":
		var data ExtraChecklistData
		if json.Unmarshal(block.Data, &data) != nil || len(data.Items) == 0 {
			return ""
		}
		var lines []string
		if data.Title != "" {
			lines = append(lines, fmt.Sprintf("☑️ **%s**", data.Title))
		}
		if data.Description != "" {
			lines = append(lines, data.Description)
		}
		for _, item := range data.Items {
			box := "☐"
			if item.Checked {
				box = "☑"
			}
			line := fmt.Sprintf("%s %s", box, item.Label)
			if item.Description != "" {
				line += " — " + item.Description
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")

	case "callout":
		var data ExtraCalloutData
		if json.Unmarshal(block.Data, &data) != nil || data.Text == "" {
			return ""
		}
		icon := "ℹ️"
		switch strings.ToLower(data.Variant) {
		case "success":
			icon = "✅"
		case "warning":
			icon = "⚠️"
		case "error":
			icon = "❌"
		}
		if data.Title != "" {
			return fmt.Sprintf("%s **%s**\n%s", icon, data.Title, data.Text)
		}
		return fmt.Sprintf("%s %s", icon, data.Text)

	case "summary":
		var data ExtraSummaryData
		if json.Unmarshal(block.Data, &data) != nil || len(data.Items) == 0 {
			return ""
		}
		var lines []string
		if data.Title != "" {
			lines = append(lines, fmt.Sprintf("📊 **%s**", data.Title))
		}
		if data.Subtitle != "" {
			lines = append(lines, data.Subtitle)
		}
		for _, item := range data.Items {
			lines = append(lines, fmt.Sprintf("• **%s**: %s", item.Label, item.Value))
		}
		return strings.Join(lines, "\n")

	case "link_preview":
		var data ExtraLinkPreviewData
		if json.Unmarshal(block.Data, &data) != nil || data.URL == "" {
			return ""
		}
		var lines []string
		if data.Title != "" && data.Title != data.URL {
			lines = append(lines, fmt.Sprintf("🔗 **%s**", data.Title))
		}
		if data.Description != "" {
			lines = append(lines, data.Description)
		}
		lines = append(lines, data.URL)
		return strings.Join(lines, "\n")

	default:
		return ""
	}
}
