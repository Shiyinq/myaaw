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

type ToolConfig struct {
	FunctionCallingConfig FunctionCallingConfig `json:"function_calling_config"`
}

type GemeniRequest struct {
	Contents          []GeminiContent          `json:"contents"`
	SystemInstruction *GeminiContent           `json:"systemInstruction,omitempty"`
	ToolConfig        *ToolConfig              `json:"toolConfig,omitempty"`
	Tools             []map[string]interface{} `json:"tools,omitempty"`
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
	for _, message := range messages {
		contentStr, ok := message.Content.(string)

		role := message.Role
		if role == "system" {
			continue
		}

		if role == "assistant" {
			role = "model"
		}

		// Gemini API only accepts "user" and "model" roles
		// "tool" responses should be sent as "user" with FunctionResponse
		if role == "tool" {
			role = "user"
		}

		var content GeminiContent
		if contentStr != "" && ok {
			content = GeminiContent{
				Parts: []GeminiPart{
					{
						Text: contentStr,
					},
				},
				Role: role,
			}

			if message.Images != nil {
				image := &GeminiInlineData{
					MimeType: "image/jpeg",
					Data:     message.Images[0],
				}
				content.Parts = append(content.Parts, GeminiPart{InlineData: image})
			}

			contents = append(contents, content)
		} else {
			// Check if it's a ToolCall message from Assistant which Gemini expects?

			geminiPart, ok := message.Content.(GeminiPart)
			if ok {
				content = GeminiContent{
					Role: role,
					Parts: []GeminiPart{
						{
							FunctionCall:     geminiPart.FunctionCall,
							FunctionResponse: geminiPart.FunctionResponse,
						},
					},
				}
				contents = append(contents, content)
			}

			// If message has ToolCalls, we should add them as FunctionCall parts (if role is model)
			if len(message.ToolCalls) > 0 && role == "model" {
				// Reconstruct FunctionCall parts from ToolCalls for history
				var parts []GeminiPart
				// Also include text if any?
				if contentStr != "" {
					parts = append(parts, GeminiPart{Text: contentStr})
				}

				for _, tc := range message.ToolCalls {
					// Arguments needs to be map[string]interface{}.

					argsMap, ok := tc.Function.Arguments.(map[string]interface{})
					if !ok {
						// If it's not a map, maybe string?
						if strArgs, isStr := tc.Function.Arguments.(string); isStr {
							_ = json.Unmarshal([]byte(strArgs), &argsMap)
						}
					}

					parts = append(parts, GeminiPart{
						FunctionCall: &GeminiFunctionCall{
							Name: tc.Function.Name,
							Args: argsMap,
						},
					})
				}

				content = GeminiContent{
					Role:  role,
					Parts: parts,
				}
				contents = append(contents, content)
			}
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
	var toolCalls []ToolCall

	for _, part := range content.Parts {
		if part.Text != "" {
			textParts = append(textParts, part.Text)
		}
		if part.FunctionCall != nil {
			toolCalls = append(toolCalls, ToolCall{
				Type: "function",
				Function: FunctionCall{
					Name:      part.FunctionCall.Name,
					Arguments: part.FunctionCall.Args,
				},
			})
		}
	}

	msg := Message{
		Role:      role,
		ToolCalls: toolCalls,
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

	request := GemeniRequest{
		Contents: MessagesToContents(messages),
		ToolConfig: &ToolConfig{
			FunctionCallingConfig: FunctionCallingConfig{
				Mode: "AUTO",
			},
		},
		Tools: []map[string]interface{}{
			{
				"function_declarations": g.getToolsTransform(),
			},
		},
	}
	if len(messages) > 0 && messages[0].Role == "system" {
		systemText := messages[0].Content.(string)

		request.SystemInstruction = &GeminiContent{
			Parts: []GeminiPart{
				{
					Text: systemText,
				},
			},
			Role: "user",
		}
	}

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

	request := GemeniRequest{
		Contents: MessagesToContents(messages),
		ToolConfig: &ToolConfig{
			FunctionCallingConfig: FunctionCallingConfig{
				Mode: "AUTO",
			},
		},
		Tools: []map[string]interface{}{
			{
				"function_declarations": g.getToolsTransform(),
			},
		},
	}

	if len(messages) > 0 && messages[0].Role == "system" {
		systemText := messages[0].Content.(string)
		request.SystemInstruction = &GeminiContent{
			Parts: []GeminiPart{
				{
					Text: systemText,
				},
			},
			Role: "user",
		}
	}

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
		if !(strings.Contains(model.Name, "1.0") || strings.Contains(model.Name, "gemini-pro") || strings.Contains(model.Name, "exp")) {
			for _, method := range model.SupportedGenerationMethods {
				if method == "generateContent" {
					models = append(models, model.Name)
				}
			}
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
