package extramessages

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"myaaw/internal/agent/tools"
	"myaaw/internal/agentdir"
	"myaaw/internal/attachment"
	"myaaw/internal/provider"
)

const (
	maxBlocksPerCall    = 10
	maxMediaItems       = 10
	maxCarouselCards    = 10
	maxButtonsPerBlock  = 20
	maxQuickReplies     = 12
	maxFormFields       = 20
	maxChecklistItems   = 25
	maxSummaryItems     = 20
	maxLocalBytesForMsg = 20 << 20 // 20 MiB cap for Telegram/Discord byte uploads
)

func init() {
	tools.RegisterBuiltin("extra_messages", New())
}

// Tool implements the built-in `extra_messages` tool.
type Tool struct {
	rootsFn func() attachment.Roots
}

// New creates an extra_messages tool using the active agent's attachment roots.
func New() *Tool {
	return &Tool{rootsFn: defaultRoots}
}

// NewWithRoots creates an extra_messages tool with custom attachment roots (for tests).
func NewWithRoots(roots attachment.Roots) *Tool {
	return &Tool{rootsFn: func() attachment.Roots { return roots }}
}

func defaultRoots() attachment.Roots {
	name := agentdir.Active()
	home, _ := agentdir.Home(name)
	user, _ := agentdir.UserDir(name)
	skills, _ := agentdir.SkillsDir(name)
	jobs, _ := agentdir.JobsDir(name)
	return attachment.Roots{Home: home, User: user, Skills: skills, Jobs: jobs}
}

func (t *Tool) ExecutionMode() tools.ExecutionMode {
	return tools.ExecutionSequential
}

func (t *Tool) CallTool(arguments string, ctx *tools.ToolsContext) string {
	return t.CallToolStructured(context.Background(), arguments, ctx).Text
}

func (t *Tool) ToolDefinition() []byte {
	return []byte(`{
		"type": "function",
		"function": {
			"name": "extra_messages",
			"description": "Present rich display or interactive UI blocks alongside your reply: multi-file media galleries (images, videos, audio, PDFs, files from online URLs or local agent paths), carousels, clickable action/link buttons, quick-reply chips, dynamic forms, checklists, summary cards, callouts, and link previews. Display blocks only render content; interactive blocks (buttons with type='action', quick_replies, form, checklist) capture user input, lock after submission, and send a structured <interaction> turn back to you. Never pass user_id or conversation_id; delivery is automatically bound to the current chat session.",
			"parameters": {
				"type": "object",
				"properties": {
					"blocks": {
						"type": "array",
						"minItems": 1,
						"maxItems": 10,
						"description": "Ordered list of display or interactive UI blocks to render.",
						"items": {
							"type": "object",
							"properties": {
								"id": {
									"type": "string",
									"description": "Optional stable block ID (auto-generated if omitted)."
								},
								"type": {
									"type": "string",
									"enum": ["media", "carousel", "buttons", "quick_replies", "form", "checklist", "callout", "summary", "link_preview"],
									"description": "Block kind. Display-only: media, callout, summary, link_preview (and carousel with only link buttons). Interactive: buttons (action), quick_replies, form, checklist (and carousel with action buttons)."
								},
								"single_use": {
									"type": "boolean",
									"description": "For interactive blocks: lock after the first click/submission (default true)."
								},
								"data": {
									"type": "object",
									"description": "Type-specific payload:\n- media: {layout?: 'grid'|'carousel'|'list', caption?, items: [{url?, path?, kind?: 'image'|'video'|'audio'|'pdf'|'file', title?, caption?, file_name?, mime_type?}]}\n- carousel: {cards: [{id?, title, subtitle?, description?, image_url?, image_path?, badges?: string[], buttons?: [{id?, label, type: 'action'|'link', value?, url?, style?: 'primary'|'secondary'|'danger', confirm?}]}]}\n- buttons: {prompt?, layout?: 'row'|'stack', buttons: [{id?, label, type: 'action'|'link', value?, url?, style?: 'primary'|'secondary'|'danger', confirm?}]}\n- quick_replies: {prompt?, options: [{id?, label, value?}]}\n- form: {id?, title, description?, submit_label?, fields: [{name, label, type: 'text'|'textarea'|'number'|'select'|'multiselect'|'checkbox'|'switch'|'date'|'time'|'radio', required?, placeholder?, default?, help?, min?, max?, options?: [{label, value}]}]}\n- checklist: {title?, description?, submit_label?, items: [{id?, label, checked?, description?}]}\n- callout: {variant?: 'info'|'success'|'warning'|'error', title?, text}\n- summary: {title?, subtitle?, items: [{label, value, tone?: 'default'|'positive'|'warning'|'danger'}]}\n- link_preview: {url, title, description?, domain?, image_url?}"
								}
							},
							"required": ["type", "data"]
						}
					}
				},
				"required": ["blocks"]
			}
		}
	}`)
}

type callArgs struct {
	Blocks []rawBlock `json:"blocks"`
}

type rawBlock struct {
	ID        string          `json:"id,omitempty"`
	Type      string          `json:"type"`
	SingleUse *bool           `json:"single_use,omitempty"`
	Data      json.RawMessage `json:"data"`
}

func (t *Tool) CallToolStructured(_ context.Context, arguments string, tctx *tools.ToolsContext) tools.StructuredResult {
	var args callArgs
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return tools.StructuredResult{Text: fmt.Sprintf("Error: invalid extra_messages arguments: %v", err)}
	}
	if len(args.Blocks) == 0 {
		return tools.StructuredResult{Text: "Error: extra_messages requires at least 1 block in blocks[]"}
	}
	if len(args.Blocks) > maxBlocksPerCall {
		return tools.StructuredResult{Text: fmt.Sprintf("Error: extra_messages accepts at most %d blocks per call", maxBlocksPerCall)}
	}

	roots := t.rootsFn()
	channelName := ""
	if tctx != nil {
		channelName = strings.ToLower(strings.TrimSpace(tctx.Channel))
	}
	isMessaging := channelName == "telegram" || channelName == "discord"

	outBlocks := make([]provider.ExtraBlock, 0, len(args.Blocks))
	var messagingFiles []tools.FileAttachment
	summaryKinds := make([]string, 0, len(args.Blocks))
	var warnings []string

	for idx, rb := range args.Blocks {
		blockType := strings.ToLower(strings.TrimSpace(rb.Type))
		blockID := strings.TrimSpace(rb.ID)
		if blockID == "" {
			blockID = "blk_" + shortID()
		}

		var (
			normalizedData any
			interactive    bool
			localFiles     []tools.FileAttachment
			kindSummary    string
			err            error
		)

		switch blockType {
		case "media":
			normalizedData, localFiles, kindSummary, err = normalizeMedia(rb.Data, roots, isMessaging)
		case "carousel":
			normalizedData, interactive, localFiles, kindSummary, err = normalizeCarousel(rb.Data, roots, isMessaging)
		case "buttons":
			normalizedData, interactive, kindSummary, err = normalizeButtons(rb.Data)
		case "quick_replies":
			normalizedData, interactive, kindSummary, err = normalizeQuickReplies(rb.Data)
		case "form":
			normalizedData, interactive, kindSummary, err = normalizeForm(rb.Data)
		case "checklist":
			normalizedData, interactive, kindSummary, err = normalizeChecklist(rb.Data)
		case "callout":
			normalizedData, kindSummary, err = normalizeCallout(rb.Data)
		case "summary":
			normalizedData, kindSummary, err = normalizeSummary(rb.Data)
		case "link_preview":
			normalizedData, kindSummary, err = normalizeLinkPreview(rb.Data)
		default:
			err = fmt.Errorf("unsupported block type %q", rb.Type)
		}

		if err != nil {
			return tools.StructuredResult{
				Text: fmt.Sprintf("Error: block[%d] (%s): %v", idx, blockType, err),
			}
		}

		rawNormalized, err := json.Marshal(normalizedData)
		if err != nil {
			return tools.StructuredResult{Text: fmt.Sprintf("Error: marshal block[%d]: %v", idx, err)}
		}

		block := provider.ExtraBlock{
			ID:          blockID,
			Type:        blockType,
			Interactive: interactive,
			SingleUse:   rb.SingleUse,
			Data:        rawNormalized,
		}
		if interactive {
			block.State = &provider.InteractionState{Status: "pending"}
		}

		outBlocks = append(outBlocks, block)
		messagingFiles = append(messagingFiles, localFiles...)
		summaryKinds = append(summaryKinds, kindSummary)

		if channelName == "telegram" && blockType == "form" {
			warnings = append(warnings, "form will display natively in WebUI and degrade to text on Telegram")
		}
	}

	extraMsg := provider.ExtraMessage{
		ID:        "em_" + shortID(),
		Blocks:    outBlocks,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Queued extra_messages (%s) with %d block(s): %s.", extraMsg.ID, len(outBlocks), strings.Join(summaryKinds, ", "))
	if channelName != "" {
		fmt.Fprintf(&b, " Active channel: %s.", channelName)
	}
	if len(warnings) > 0 {
		fmt.Fprintf(&b, " Note: %s.", strings.Join(warnings, "; "))
	}

	return tools.StructuredResult{
		Text:          b.String(),
		Attachments:   messagingFiles,
		ExtraMessages: []provider.ExtraMessage{extraMsg},
	}
}

func shortID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
}

func validateHTTPURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("url is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("invalid url %q", trimmed)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("url scheme %q is not allowed (only http/https)", parsed.Scheme)
	}
	return trimmed, nil
}

func resolveLocalMedia(rawPath string, roots attachment.Roots, readBytes bool) (attachment.Item, *tools.FileAttachment, error) {
	item, ok := attachment.Resolve(strings.TrimSpace(rawPath), roots)
	if !ok {
		return attachment.Item{}, nil, fmt.Errorf("local path %q could not be resolved inside allowed agent roots", rawPath)
	}
	var fileAtt *tools.FileAttachment
	if readBytes && item.Size <= maxLocalBytesForMsg {
		if data, err := os.ReadFile(item.AbsPath); err == nil {
			fileAtt = &tools.FileAttachment{
				FileName: item.Name,
				MimeType: item.Mime,
				Data:     data,
				Path:     item.Virtual,
			}
		}
	}
	return item, fileAtt, nil
}

func inferMediaKindFromURL(rawURL, mimeType string) string {
	lowerMime := strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(lowerMime, "image/"):
		return "image"
	case strings.HasPrefix(lowerMime, "video/"):
		return "video"
	case strings.HasPrefix(lowerMime, "audio/"):
		return "audio"
	case lowerMime == "application/pdf":
		return "pdf"
	}
	parsed, err := url.Parse(rawURL)
	pathPart := rawURL
	if err == nil && parsed.Path != "" {
		pathPart = parsed.Path
	}
	ext := strings.ToLower(filepath.Ext(pathPart))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".avif", ".bmp":
		return "image"
	case ".mp4", ".m4v", ".webm", ".mov", ".mkv", ".ogv":
		return "video"
	case ".mp3", ".wav", ".m4a", ".aac", ".flac", ".ogg", ".opus":
		return "audio"
	case ".pdf":
		return "pdf"
	default:
		return "file"
	}
}

func fileNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Path != "" {
		base := filepath.Base(parsed.Path)
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	return "file"
}

// ---------------------------------------------------------------------------
// Block normalizers
// ---------------------------------------------------------------------------

type mediaItem struct {
	URL      string `json:"url,omitempty"`
	Path     string `json:"path,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Title    string `json:"title,omitempty"`
	Caption  string `json:"caption,omitempty"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type mediaBlockData struct {
	Layout  string      `json:"layout,omitempty"`
	Caption string      `json:"caption,omitempty"`
	Items   []mediaItem `json:"items"`
}

func normalizeMedia(raw json.RawMessage, roots attachment.Roots, isMessaging bool) (mediaBlockData, []tools.FileAttachment, string, error) {
	var data mediaBlockData
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, nil, "", fmt.Errorf("invalid media data: %w", err)
	}
	if len(data.Items) == 0 {
		return data, nil, "", fmt.Errorf("media block requires at least 1 item")
	}
	if len(data.Items) > maxMediaItems {
		return data, nil, "", fmt.Errorf("media block allows at most %d items", maxMediaItems)
	}
	layout := strings.ToLower(strings.TrimSpace(data.Layout))
	if layout != "grid" && layout != "carousel" && layout != "list" {
		layout = "grid"
	}
	data.Layout = layout
	data.Caption = strings.TrimSpace(data.Caption)

	var files []tools.FileAttachment
	for i := range data.Items {
		item := &data.Items[i]
		item.URL = strings.TrimSpace(item.URL)
		item.Path = strings.TrimSpace(item.Path)
		item.Title = strings.TrimSpace(item.Title)
		item.Caption = strings.TrimSpace(item.Caption)
		item.FileName = strings.TrimSpace(item.FileName)
		item.MimeType = strings.TrimSpace(item.MimeType)
		item.Kind = strings.ToLower(strings.TrimSpace(item.Kind))

		if item.URL == "" && item.Path == "" {
			return data, nil, "", fmt.Errorf("media item[%d] must set either url or path", i)
		}
		if item.Path != "" {
			resolved, fileAtt, err := resolveLocalMedia(item.Path, roots, isMessaging)
			if err != nil {
				return data, nil, "", fmt.Errorf("media item[%d]: %w", i, err)
			}
			item.Path = resolved.Virtual
			if item.FileName == "" {
				item.FileName = resolved.Name
			}
			if item.MimeType == "" {
				item.MimeType = resolved.Mime
			}
			if item.Kind == "" {
				item.Kind = string(resolved.Kind)
			}
			item.Size = resolved.Size
			item.URL = ""
			if fileAtt != nil {
				files = append(files, *fileAtt)
			}
		} else {
			cleanURL, err := validateHTTPURL(item.URL)
			if err != nil {
				return data, nil, "", fmt.Errorf("media item[%d]: %w", i, err)
			}
			item.URL = cleanURL
			if item.Kind == "" {
				item.Kind = inferMediaKindFromURL(cleanURL, item.MimeType)
			}
			if item.FileName == "" {
				item.FileName = fileNameFromURL(cleanURL)
			}
		}
		switch item.Kind {
		case "image", "video", "audio", "pdf", "file":
		default:
			item.Kind = "file"
		}
	}

	return data, files, fmt.Sprintf("media[%d]", len(data.Items)), nil
}

type extraButton struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Type    string `json:"type"` // "action" | "link"
	Value   string `json:"value,omitempty"`
	URL     string `json:"url,omitempty"`
	Style   string `json:"style,omitempty"` // "primary" | "secondary" | "danger"
	Confirm string `json:"confirm,omitempty"`
}

func normalizeButtonList(buttons []extraButton, maxCount int) ([]extraButton, bool, error) {
	if len(buttons) > maxCount {
		return nil, false, fmt.Errorf("at most %d buttons are allowed", maxCount)
	}
	hasAction := false
	seenID := make(map[string]bool, len(buttons))
	out := make([]extraButton, 0, len(buttons))
	for i, btn := range buttons {
		btn.Label = strings.TrimSpace(btn.Label)
		if btn.Label == "" {
			return nil, false, fmt.Errorf("button[%d] label is required", i)
		}
		btn.Type = strings.ToLower(strings.TrimSpace(btn.Type))
		if btn.Type == "" {
			if strings.TrimSpace(btn.URL) != "" {
				btn.Type = "link"
			} else {
				btn.Type = "action"
			}
		}
		switch btn.Type {
		case "link":
			cleanURL, err := validateHTTPURL(btn.URL)
			if err != nil {
				return nil, false, fmt.Errorf("button[%d]: %w", i, err)
			}
			btn.URL = cleanURL
		case "action":
			hasAction = true
			btn.URL = ""
			if strings.TrimSpace(btn.Value) == "" {
				btn.Value = btn.Label
			}
		default:
			return nil, false, fmt.Errorf("button[%d] invalid type %q", i, btn.Type)
		}
		btn.ID = strings.TrimSpace(btn.ID)
		if btn.ID == "" {
			btn.ID = fmt.Sprintf("btn_%d_%s", i+1, shortID()[:6])
		}
		if seenID[btn.ID] {
			btn.ID = fmt.Sprintf("%s_%d", btn.ID, i+1)
		}
		seenID[btn.ID] = true

		btn.Style = strings.ToLower(strings.TrimSpace(btn.Style))
		if btn.Style != "primary" && btn.Style != "secondary" && btn.Style != "danger" {
			btn.Style = "secondary"
		}
		btn.Confirm = strings.TrimSpace(btn.Confirm)
		out = append(out, btn)
	}
	return out, hasAction, nil
}

type buttonsBlockData struct {
	Prompt  string        `json:"prompt,omitempty"`
	Layout  string        `json:"layout,omitempty"` // "row" | "stack"
	Buttons []extraButton `json:"buttons"`
}

func normalizeButtons(raw json.RawMessage) (buttonsBlockData, bool, string, error) {
	var data buttonsBlockData
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, false, "", fmt.Errorf("invalid buttons data: %w", err)
	}
	if len(data.Buttons) == 0 {
		return data, false, "", fmt.Errorf("buttons block requires at least 1 button")
	}
	btns, hasAction, err := normalizeButtonList(data.Buttons, maxButtonsPerBlock)
	if err != nil {
		return data, false, "", err
	}
	data.Buttons = btns
	data.Prompt = strings.TrimSpace(data.Prompt)
	layout := strings.ToLower(strings.TrimSpace(data.Layout))
	if layout != "stack" {
		layout = "row"
	}
	data.Layout = layout
	return data, hasAction, fmt.Sprintf("buttons[%d]", len(btns)), nil
}

type quickReplyOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
}

type quickRepliesBlockData struct {
	Prompt  string             `json:"prompt,omitempty"`
	Options []quickReplyOption `json:"options"`
}

func normalizeQuickReplies(raw json.RawMessage) (quickRepliesBlockData, bool, string, error) {
	var data quickRepliesBlockData
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, false, "", fmt.Errorf("invalid quick_replies data: %w", err)
	}
	if len(data.Options) == 0 {
		return data, false, "", fmt.Errorf("quick_replies requires at least 1 option")
	}
	if len(data.Options) > maxQuickReplies {
		return data, false, "", fmt.Errorf("quick_replies allows at most %d options", maxQuickReplies)
	}
	data.Prompt = strings.TrimSpace(data.Prompt)
	for i := range data.Options {
		opt := &data.Options[i]
		opt.Label = strings.TrimSpace(opt.Label)
		if opt.Label == "" {
			return data, false, "", fmt.Errorf("option[%d] label is required", i)
		}
		opt.ID = strings.TrimSpace(opt.ID)
		if opt.ID == "" {
			opt.ID = fmt.Sprintf("opt_%d_%s", i+1, shortID()[:6])
		}
		opt.Value = strings.TrimSpace(opt.Value)
		if opt.Value == "" {
			opt.Value = opt.Label
		}
	}
	return data, true, fmt.Sprintf("quick_replies[%d]", len(data.Options)), nil
}

type carouselCard struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Subtitle    string        `json:"subtitle,omitempty"`
	Description string        `json:"description,omitempty"`
	ImageURL    string        `json:"image_url,omitempty"`
	ImagePath   string        `json:"image_path,omitempty"`
	Badges      []string      `json:"badges,omitempty"`
	Buttons     []extraButton `json:"buttons,omitempty"`
}

type carouselBlockData struct {
	Cards []carouselCard `json:"cards"`
}

func normalizeCarousel(raw json.RawMessage, roots attachment.Roots, isMessaging bool) (carouselBlockData, bool, []tools.FileAttachment, string, error) {
	var data carouselBlockData
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, false, nil, "", fmt.Errorf("invalid carousel data: %w", err)
	}
	if len(data.Cards) == 0 {
		return data, false, nil, "", fmt.Errorf("carousel requires at least 1 card")
	}
	if len(data.Cards) > maxCarouselCards {
		return data, false, nil, "", fmt.Errorf("carousel allows at most %d cards", maxCarouselCards)
	}

	var (
		hasAction bool
		files     []tools.FileAttachment
	)
	for i := range data.Cards {
		card := &data.Cards[i]
		card.Title = strings.TrimSpace(card.Title)
		if card.Title == "" {
			return data, false, nil, "", fmt.Errorf("card[%d] title is required", i)
		}
		card.ID = strings.TrimSpace(card.ID)
		if card.ID == "" {
			card.ID = fmt.Sprintf("card_%d_%s", i+1, shortID()[:6])
		}
		card.Subtitle = strings.TrimSpace(card.Subtitle)
		card.Description = strings.TrimSpace(card.Description)
		card.ImageURL = strings.TrimSpace(card.ImageURL)
		card.ImagePath = strings.TrimSpace(card.ImagePath)

		if card.ImagePath != "" {
			resolved, fileAtt, err := resolveLocalMedia(card.ImagePath, roots, isMessaging)
			if err != nil {
				return data, false, nil, "", fmt.Errorf("card[%d]: %w", i, err)
			}
			card.ImagePath = resolved.Virtual
			card.ImageURL = ""
			if fileAtt != nil {
				files = append(files, *fileAtt)
			}
		} else if card.ImageURL != "" {
			cleanURL, err := validateHTTPURL(card.ImageURL)
			if err != nil {
				return data, false, nil, "", fmt.Errorf("card[%d]: %w", i, err)
			}
			card.ImageURL = cleanURL
		}

		if len(card.Buttons) > 0 {
			btns, cardAction, err := normalizeButtonList(card.Buttons, 4)
			if err != nil {
				return data, false, nil, "", fmt.Errorf("card[%d]: %w", i, err)
			}
			card.Buttons = btns
			if cardAction {
				hasAction = true
			}
		}
	}

	return data, hasAction, files, fmt.Sprintf("carousel[%d]", len(data.Cards)), nil
}

type formOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type formField struct {
	Name        string       `json:"name"`
	Label       string       `json:"label"`
	Type        string       `json:"type"`
	Required    bool         `json:"required,omitempty"`
	Placeholder string       `json:"placeholder,omitempty"`
	Default     any          `json:"default,omitempty"`
	Help        string       `json:"help,omitempty"`
	Min         *float64     `json:"min,omitempty"`
	Max         *float64     `json:"max,omitempty"`
	Options     []formOption `json:"options,omitempty"`
}

type formBlockData struct {
	ID          string      `json:"id"`
	Title       string      `json:"title"`
	Description string      `json:"description,omitempty"`
	SubmitLabel string      `json:"submit_label,omitempty"`
	Fields      []formField `json:"fields"`
}

var allowedFieldTypes = map[string]bool{
	"text":        true,
	"textarea":    true,
	"number":      true,
	"select":      true,
	"multiselect": true,
	"checkbox":    true,
	"switch":      true,
	"date":        true,
	"time":        true,
	"radio":       true,
}

func normalizeForm(raw json.RawMessage) (formBlockData, bool, string, error) {
	var data formBlockData
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, false, "", fmt.Errorf("invalid form data: %w", err)
	}
	data.Title = strings.TrimSpace(data.Title)
	if data.Title == "" {
		return data, false, "", fmt.Errorf("form title is required")
	}
	if len(data.Fields) == 0 {
		return data, false, "", fmt.Errorf("form requires at least 1 field")
	}
	if len(data.Fields) > maxFormFields {
		return data, false, "", fmt.Errorf("form allows at most %d fields", maxFormFields)
	}
	data.ID = strings.TrimSpace(data.ID)
	if data.ID == "" {
		data.ID = "form_" + shortID()[:8]
	}
	data.Description = strings.TrimSpace(data.Description)
	data.SubmitLabel = strings.TrimSpace(data.SubmitLabel)

	seenName := make(map[string]bool, len(data.Fields))
	for i := range data.Fields {
		f := &data.Fields[i]
		f.Name = strings.TrimSpace(f.Name)
		f.Label = strings.TrimSpace(f.Label)
		f.Type = strings.ToLower(strings.TrimSpace(f.Type))
		if f.Name == "" {
			return data, false, "", fmt.Errorf("field[%d] name is required", i)
		}
		if seenName[f.Name] {
			return data, false, "", fmt.Errorf("duplicate form field name %q", f.Name)
		}
		seenName[f.Name] = true
		if f.Label == "" {
			f.Label = f.Name
		}
		if f.Type == "" {
			f.Type = "text"
		}
		if !allowedFieldTypes[f.Type] {
			return data, false, "", fmt.Errorf("field[%d] unsupported type %q", i, f.Type)
		}
		if f.Type == "select" || f.Type == "multiselect" || f.Type == "radio" {
			if len(f.Options) == 0 {
				return data, false, "", fmt.Errorf("field %q (%s) requires options", f.Name, f.Type)
			}
			for j := range f.Options {
				f.Options[j].Label = strings.TrimSpace(f.Options[j].Label)
				f.Options[j].Value = strings.TrimSpace(f.Options[j].Value)
				if f.Options[j].Value == "" {
					f.Options[j].Value = f.Options[j].Label
				}
				if f.Options[j].Label == "" {
					f.Options[j].Label = f.Options[j].Value
				}
			}
		}
	}

	return data, true, fmt.Sprintf("form[%d]", len(data.Fields)), nil
}

type checklistItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Checked     bool   `json:"checked,omitempty"`
}

type checklistBlockData struct {
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	SubmitLabel string          `json:"submit_label,omitempty"`
	Items       []checklistItem `json:"items"`
}

func normalizeChecklist(raw json.RawMessage) (checklistBlockData, bool, string, error) {
	var data checklistBlockData
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, false, "", fmt.Errorf("invalid checklist data: %w", err)
	}
	if len(data.Items) == 0 {
		return data, false, "", fmt.Errorf("checklist requires at least 1 item")
	}
	if len(data.Items) > maxChecklistItems {
		return data, false, "", fmt.Errorf("checklist allows at most %d items", maxChecklistItems)
	}
	data.Title = strings.TrimSpace(data.Title)
	data.Description = strings.TrimSpace(data.Description)
	data.SubmitLabel = strings.TrimSpace(data.SubmitLabel)
	for i := range data.Items {
		item := &data.Items[i]
		item.Label = strings.TrimSpace(item.Label)
		if item.Label == "" {
			return data, false, "", fmt.Errorf("checklist item[%d] label is required", i)
		}
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" {
			item.ID = fmt.Sprintf("chk_%d_%s", i+1, shortID()[:6])
		}
		item.Description = strings.TrimSpace(item.Description)
	}
	return data, true, fmt.Sprintf("checklist[%d]", len(data.Items)), nil
}

type calloutBlockData struct {
	Variant string `json:"variant,omitempty"` // "info" | "success" | "warning" | "error"
	Title   string `json:"title,omitempty"`
	Text    string `json:"text"`
}

func normalizeCallout(raw json.RawMessage) (calloutBlockData, string, error) {
	var data calloutBlockData
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, "", fmt.Errorf("invalid callout data: %w", err)
	}
	data.Text = strings.TrimSpace(data.Text)
	if data.Text == "" {
		return data, "", fmt.Errorf("callout text is required")
	}
	data.Title = strings.TrimSpace(data.Title)
	v := strings.ToLower(strings.TrimSpace(data.Variant))
	if v != "success" && v != "warning" && v != "error" {
		v = "info"
	}
	data.Variant = v
	return data, "callout", nil
}

type summaryItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"` // "default" | "positive" | "warning" | "danger"
}

type summaryBlockData struct {
	Title    string        `json:"title,omitempty"`
	Subtitle string        `json:"subtitle,omitempty"`
	Items    []summaryItem `json:"items"`
}

func normalizeSummary(raw json.RawMessage) (summaryBlockData, string, error) {
	var data summaryBlockData
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, "", fmt.Errorf("invalid summary data: %w", err)
	}
	if len(data.Items) == 0 {
		return data, "", fmt.Errorf("summary requires at least 1 item")
	}
	if len(data.Items) > maxSummaryItems {
		return data, "", fmt.Errorf("summary allows at most %d items", maxSummaryItems)
	}
	data.Title = strings.TrimSpace(data.Title)
	data.Subtitle = strings.TrimSpace(data.Subtitle)
	for i := range data.Items {
		it := &data.Items[i]
		it.Label = strings.TrimSpace(it.Label)
		it.Value = strings.TrimSpace(it.Value)
		if it.Label == "" || it.Value == "" {
			return data, "", fmt.Errorf("summary item[%d] requires both label and value", i)
		}
		tone := strings.ToLower(strings.TrimSpace(it.Tone))
		if tone != "positive" && tone != "warning" && tone != "danger" {
			tone = "default"
		}
		it.Tone = tone
	}
	return data, fmt.Sprintf("summary[%d]", len(data.Items)), nil
}

type linkPreviewBlockData struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Domain      string `json:"domain,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
}

func normalizeLinkPreview(raw json.RawMessage) (linkPreviewBlockData, string, error) {
	var data linkPreviewBlockData
	if err := json.Unmarshal(raw, &data); err != nil {
		return data, "", fmt.Errorf("invalid link_preview data: %w", err)
	}
	cleanURL, err := validateHTTPURL(data.URL)
	if err != nil {
		return data, "", err
	}
	data.URL = cleanURL
	data.Title = strings.TrimSpace(data.Title)
	if data.Title == "" {
		data.Title = cleanURL
	}
	data.Description = strings.TrimSpace(data.Description)
	data.Domain = strings.TrimSpace(data.Domain)
	if data.Domain == "" {
		if parsed, err := url.Parse(cleanURL); err == nil {
			data.Domain = parsed.Hostname()
		}
	}
	if strings.TrimSpace(data.ImageURL) != "" {
		imgURL, err := validateHTTPURL(data.ImageURL)
		if err != nil {
			return data, "", fmt.Errorf("image_url: %w", err)
		}
		data.ImageURL = imgURL
	}
	return data, "link_preview", nil
}
