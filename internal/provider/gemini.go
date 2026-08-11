package provider

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"myaaw/internal/agent/tools"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type GeminiInlineData struct {
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"`
}

type GeminiFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type GeminiFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type GeminiPart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *GeminiInlineData       `json:"inline_data,omitempty"`
	FunctionCall     *GeminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
	Thought          bool                    `json:"thought,omitempty"`
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts,omitempty"`
	Role  string       `json:"role,omitempty"`
}

type GeminiSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type GeminiCandidate struct {
	Content       GeminiContent        `json:"content"`
	FinishReason  string               `json:"finishReason"`
	Index         int                  `json:"index"`
	SafetyRatings []GeminiSafetyRating `json:"safetyRatings"`
}

type GeminiTokensDetail struct {
	Modality   string `json:"modality"`
	TokenCount int    `json:"tokenCount"`
}

type GeminiUsageMetadata struct {
	PromptTokenCount        int                  `json:"promptTokenCount"`
	CandidatesTokenCount    int                  `json:"candidatesTokenCount"`
	TotalTokenCount         int                  `json:"totalTokenCount"`
	ThoughtsTokenCount      int                  `json:"thoughtsTokenCount"`
	PromptTokensDetails     []GeminiTokensDetail `json:"promptTokensDetails"`
	CandidatesTokensDetails []GeminiTokensDetail `json:"candidatesTokensDetails"`
}

type GeminiGenerateContent struct {
	Candidates    []GeminiCandidate   `json:"candidates"`
	UsageMetadata GeminiUsageMetadata `json:"usageMetadata"`
}

type FunctionCallingConfig struct {
	Mode string `json:"mode"`
}

type GemeniGenerationConfig struct {
	ThinkingConfig *GeminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type GeminiThinkingConfig struct {
	IncludeThoughts bool   `json:"includeThoughts"`
	ThinkingLevel   string `json:"thinkingLevel,omitempty"`
}

type ToolConfig struct {
	FunctionCallingConfig FunctionCallingConfig `json:"function_calling_config"`
}

type GemeniRequest struct {
	Contents          []GeminiContent          `json:"contents"`
	SystemInstruction *GeminiContent           `json:"systemInstruction,omitempty"`
	ToolConfig        *ToolConfig              `json:"toolConfig,omitempty"`
	Tools             []map[string]interface{} `json:"tools,omitempty"`
	GenerationConfig  *GemeniGenerationConfig  `json:"generationConfig,omitempty"`
}

type GeminiModel struct {
	Name                       string   `json:"name"`
	Version                    string   `json:"version"`
	DisplayName                string   `json:"displayName"`
	Description                string   `json:"description"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	Temperature                float64  `json:"temperature,omitempty"`
	TopP                       float64  `json:"topP,omitempty"`
	TopK                       int      `json:"topK,omitempty"`
	MaxTemperature             float64  `json:"maxTemperature,omitempty"`
	Thinking                   bool     `json:"thinking,omitempty"`
}

type GeminiModels struct {
	Models []GeminiModel `json:"models"`
}

type GeminiProvider struct {
	baseURL      string
	apiKey       string
	defaultModel string
}

func NewGeminiProvider(baseURL string, apiKey string, defaultModel string) LLMProvider {
	return &GeminiProvider{
		baseURL:      baseURL,
		apiKey:       apiKey,
		defaultModel: defaultModel,
	}
}

func MessagesToContents(messages []Message) []GeminiContent {
	var contents []GeminiContent

	for i, message := range messages {
		if message.Role == "system" && i == 0 {
			continue
		}

		role := message.Role
		if role == "assistant" {
			role = "model"
		} else if role == "tool" || role == "system" {
			role = "user"
		}

		var parts []GeminiPart

		// 1. Handle Thought (for model)
		if message.Thought != "" && role == "model" {
			parts = append(parts, GeminiPart{
				Text:    message.Thought,
				Thought: true,
			})
		}

		// 2. Handle Text Content
		contentStr, isStr := message.Content.(string)
		if isStr && contentStr != "" {
			parts = append(parts, GeminiPart{
				Text: contentStr,
			})
		} else if geminiPart, isPart := message.Content.(GeminiPart); isPart {
			// Backward compatibility or direct GeminiPart passing
			parts = append(parts, geminiPart)
		}

		// 3. Handle Tool Calls (for model)
		if role == "model" && len(message.ToolCalls) > 0 {
			for _, tc := range message.ToolCalls {
				var argsMap map[string]interface{}
				if tc.Function.Arguments != nil {
					switch a := tc.Function.Arguments.(type) {
					case map[string]interface{}:
						argsMap = a
					case string:
						_ = json.Unmarshal([]byte(a), &argsMap)
					}
				}

				parts = append(parts, GeminiPart{
					FunctionCall: &GeminiFunctionCall{
						Name: tc.Function.Name,
						Args: argsMap,
					},
					ThoughtSignature: tc.Function.ThoughtSignature,
				})
			}
		}

		// 4. Handle Tool Responses (for tool role)
		if message.Role == "tool" {
			// Gemini expects FunctionResponse to be matched by Name
			parts = append(parts, GeminiPart{
				FunctionResponse: &GeminiFunctionResponse{
					Name: message.Name,
					Response: map[string]interface{}{
						"result": message.Content,
					},
				},
			})
		}

		// 5. Handle Images (usually for user)
		if len(message.Images) > 0 && role == "user" {
			for _, img := range message.Images {
				mimeType := "image/jpeg" // default
				if strings.HasPrefix(img, "iVBORw0KGgo") {
					mimeType = "image/png"
				} else if strings.HasPrefix(img, "UklGR") {
					mimeType = "image/webp"
				} else if strings.HasPrefix(img, "/9j/") {
					mimeType = "image/jpeg"
				}

				parts = append(parts, GeminiPart{
					InlineData: &GeminiInlineData{
						MimeType: mimeType,
						Data:     img,
					},
				})
			}
		}

		if len(parts) == 0 {
			continue
		}

		// BUNDLING LOGIC: If the last content has the same role, merge parts.
		// This handles consecutive tool responses and user prompts (like ThoughtPrompt).
		if len(contents) > 0 && contents[len(contents)-1].Role == role {
			contents[len(contents)-1].Parts = append(contents[len(contents)-1].Parts, parts...)
		} else {
			contents = append(contents, GeminiContent{
				Role:  role,
				Parts: parts,
			})
		}
	}

	return contents
}

func contentToMessage(content GeminiContent) Message {
	role := content.Role
	if role == "model" {
		role = "assistant"
	}

	var textParts []string
	var thoughtParts []string
	var toolCalls []ToolCall

	for _, part := range content.Parts {
		if part.Thought {
			if part.Text != "" {
				text := part.Text

				// Fallback string matching for Gemini thinking payload missing the flag
				// because gemini sometimes doesn't set the thought flag to false even if is not thought part
				if strings.HasPrefix(text, "**") && strings.Contains(text, "**\n\n") && strings.HasSuffix(text, "\n\n\n") {
					thoughtParts = append(thoughtParts, text)
				} else {
					part.Thought = false
					textParts = append(textParts, text)
				}

			}
		} else if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			toolCalls = append(toolCalls, ToolCall{
				Type: "function",
				Function: FunctionCall{
					Name:             part.FunctionCall.Name,
					Arguments:        part.FunctionCall.Args,
					ThoughtSignature: part.ThoughtSignature,
				},
			})
		}
	}

	msg := Message{
		Role:      role,
		ToolCalls: toolCalls,
	}

	if len(thoughtParts) > 0 {
		msg.Thought = strings.Join(thoughtParts, "\n")
	}

	if len(textParts) > 0 {
		msg.Content = strings.Join(textParts, "\n")
	} else if len(toolCalls) == 0 {
		// Empty content?
		msg.Content = ""
	}

	return msg
}

func (g *GeminiProvider) ProviderName() string {
	return "gemini"
}

func (g *GeminiProvider) DefaultModel(modelName string) string {
	if modelName == "" {
		return g.defaultModel
	}
	return modelName
}

func (g *GeminiProvider) isThinkingModel(modelName string) bool {
	name := strings.TrimPrefix(modelName, "models/")
	thinkingModels := map[string]bool{
		"gemini-flash-latest":                   true,
		"gemini-flash-lite-latest":              true,
		"gemini-2.5-flash":                      true,
		"gemini-2.5-pro":                        true,
		"gemini-2.5-flash-lite":                 true,
		"gemini-2.5-flash-lite-preview-09-2025": true,
		"gemini-3-pro-preview":                  true,
		"gemini-3-flash-preview":                true,
		"gemini-3.1-pro-preview":                true,
	}
	return thinkingModels[name]
}

func (g *GeminiProvider) supportsTools(modelName string) bool {
	name := strings.TrimPrefix(modelName, "models/")
	supportedPrefixes := []string{
		"gemini-flash",
		"gemini-2.0-flash",
		"gemini-2.0-flash-lite",
		"gemini-2.0-pro",
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-3-pro",
		"gemini-3-flash",
		"gemini-3.1-pro",
		"gemini-3.1-flash",
	}

	for _, prefix := range supportedPrefixes {
		if strings.HasPrefix(name, prefix) {
			if strings.Contains(name, "-tts") || strings.Contains(name, "-image-generation") ||
				strings.Contains(name, "-native-audio") || strings.Contains(name, "robotics") {
				return false
			}
			return true
		}
	}
	return false
}

func (g *GeminiProvider) supportsSystemInstruction(modelName string) bool {
	name := strings.TrimPrefix(modelName, "models/")
	if strings.HasPrefix(name, "gemma-") {
		return false
	}
	if strings.HasPrefix(name, "gemini-1.0") || name == "gemini-pro" || name == "gemini-pro-vision" {
		return false
	}
	return g.supportsTools(modelName)
}

func (g *GeminiProvider) buildRequest(modelName string, messages []Message) GemeniRequest {
	fullModel := g.DefaultModel(modelName)
	request := GemeniRequest{
		Contents: MessagesToContents(messages),
	}

	if g.supportsTools(fullModel) {
		request.ToolConfig = &ToolConfig{
			FunctionCallingConfig: FunctionCallingConfig{
				Mode: "AUTO",
			},
		}
		request.Tools = []map[string]any{
			{
				"function_declarations": g.getToolsTransform(),
			},
		}
	}

	if g.isThinkingModel(fullModel) {
		request.GenerationConfig = &GemeniGenerationConfig{
			ThinkingConfig: &GeminiThinkingConfig{
				IncludeThoughts: true,
			},
		}
	}

	if len(messages) > 0 && messages[0].Role == "system" {
		systemText := messages[0].Content.(string)

		if g.supportsSystemInstruction(fullModel) {
			request.SystemInstruction = &GeminiContent{
				Parts: []GeminiPart{
					{
						Text: systemText,
					},
				},
				Role: "user",
			}
		} else {
			if len(request.Contents) > 0 && len(request.Contents[0].Parts) > 0 {
				request.Contents[0].Parts[0].Text = fmt.Sprintf("System: %s\n\n%s", systemText, request.Contents[0].Parts[0].Text)
			}
		}
	}

	return request
}

func (g *GeminiProvider) getToolsTransform() []map[string]interface{} {
	originalTools := tools.GetTools()
	if originalTools == nil {
		return nil
	}

	var flattenedTools []map[string]interface{}
	for _, tool := range originalTools {
		if functionValue, ok := tool["function"].(map[string]interface{}); ok {
			flattenedTools = append(flattenedTools, functionValue)
		}
	}

	return flattenedTools
}

func (g *GeminiProvider) Chat(modelName string, messages []Message) (Message, error) {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := g.buildRequest(modelName, messages)

	var response GeminiGenerateContent
	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetResult(&response).
		Post(g.baseURL + fmt.Sprintf("/v1beta/%s:generateContent?key=%s", g.DefaultModel(modelName), g.apiKey))

	if res.StatusCode() != 200 {
		return Message{}, fmt.Errorf("error fetching response: %v", res.String())
	}

	// Check safety
	if len(response.Candidates) > 0 && response.Candidates[0].FinishReason == "SAFETY" {
		return Message{}, fmt.Errorf("SAFETY")
	}

	if len(response.Candidates) == 0 {
		return Message{}, fmt.Errorf("no candidates returned")
	}

	finalMsg := contentToMessage(response.Candidates[0].Content)

	// Populate Usage
	finalMsg.Usage = g.extractUsage(response.UsageMetadata)

	return finalMsg, nil
}

func (g *GeminiProvider) ChatStream(modelName string, messages []Message, callback func(Message) error) error {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := g.buildRequest(modelName, messages)

	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(g.baseURL + fmt.Sprintf("/v1beta/%s:streamGenerateContent?key=%s", g.DefaultModel(modelName), g.apiKey))

	if err != nil {
		return fmt.Errorf("error in stream request: %w", err)
	}

	if res == nil {
		return fmt.Errorf("empty response")
	}

	defer res.RawBody().Close()

	reader := bufio.NewReader(res.RawBody())
	var response GeminiGenerateContent
	bufferJSON := ""
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading stream: %w", err)
		}

		line = strings.TrimSpace(line)
		if line != "," {
			bufferJSON += line
		}

		bufferJSON = strings.TrimPrefix(bufferJSON, "[")
		err = json.Unmarshal([]byte(bufferJSON), &response)
		if err != nil {
			continue
		}

		if res.StatusCode() != 200 {
			return fmt.Errorf("error fetching stream response: %v", bufferJSON)
		}

		if len(response.Candidates) > 0 {
			partialMessage := contentToMessage(response.Candidates[0].Content)
			err = callback(partialMessage)
			if err != nil {
				return fmt.Errorf("error in callback: %w", err)
			}
		}

		// Check for UsageMetadata (usually at the end or with candidates)
		if response.UsageMetadata.TotalTokenCount > 0 {
			usageMsg := Message{
				Role:  "assistant",
				Usage: g.extractUsage(response.UsageMetadata),
			}

			// Send usage update via callback
			if err := callback(usageMsg); err != nil {
				return fmt.Errorf("error in callback sending usage: %w", err)
			}
		}

		bufferJSON = ""
	}

	return nil
}

func (g *GeminiProvider) extractUsage(usage GeminiUsageMetadata) Usage {
	finalUsage := Usage{
		PromptTokens:     usage.PromptTokenCount,
		CompletionTokens: usage.CandidatesTokenCount,
		TotalTokens:      usage.TotalTokenCount,
		ThoughtsTokens:   usage.ThoughtsTokenCount,
	}

	for _, detail := range usage.PromptTokensDetails {
		finalUsage.Details = append(finalUsage.Details, UsageDetail{
			Modality:   detail.Modality,
			TokenCount: detail.TokenCount,
		})
	}

	for _, detail := range usage.CandidatesTokensDetails {
		finalUsage.Details = append(finalUsage.Details, UsageDetail{
			Modality:   detail.Modality,
			TokenCount: detail.TokenCount,
		})
	}

	return finalUsage
}

func (g *GeminiProvider) Models() ([]string, error) {
	response, err := g.geminiModels()
	if err != nil {
		return nil, err
	}

	var models []string
	for _, model := range response.Models {
		name := strings.TrimPrefix(model.Name, "models/")

		supportsGenerate := false
		for _, method := range model.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGenerate = true
				break
			}
		}
		if !supportsGenerate {
			continue
		}

		if strings.Contains(name, "-tts") || strings.Contains(name, "-image") ||
			strings.Contains(name, "-generation") || strings.Contains(name, "native-audio") ||
			strings.Contains(name, "robotics") || strings.Contains(name, "computer-use") ||
			strings.Contains(name, "aqa") || strings.Contains(name, "embedding") ||
			strings.Contains(name, "imagen") || strings.Contains(name, "veo") {
			continue
		}

		if strings.HasPrefix(name, "deep-research") || strings.HasPrefix(name, "gemma-") {
			continue
		}

		isFlash := strings.Contains(name, "flash")
		isV2Plus := false
		prefixes := []string{"gemini-2.", "gemini-3.", "gemini-3-"}
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				isV2Plus = true
				break
			}
		}

		if isFlash || isV2Plus {
			models = append(models, model.Name)
		}
	}

	return models, nil
}

func (g *GeminiProvider) geminiModels() (*GeminiModels, error) {
	client := resty.New()

	var response GeminiModels
	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetResult(&response).
		Get(g.baseURL + fmt.Sprintf("/v1beta/models?key=%s", g.apiKey))

	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("error fetching gemini models: %s", res.String())
	}

	return &response, nil
}
