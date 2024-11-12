package provider

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type GeminiInlineData struct {
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"`
}

type GeminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *GeminiInlineData `json:"inline_data,omitempty"`
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

type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type GeminiGenerateContent struct {
	Candidates    []GeminiCandidate   `json:"candidates"`
	UsageMetadata GeminiUsageMetadata `json:"usageMetadata"`
}

type GemeniRequest struct {
	Contents          []GeminiContent `json:"contents"`
	SystemInstruction *GeminiContent  `json:"systemInstruction,omitempty"`
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
		if !ok {
			log.Println("content is not a string, skipping this message")
			continue
		}

		role := message.Role
		if role == "system" {
			continue
		}

		if role == "assistant" {
			role = "model"
		}

		var content GeminiContent
		if contentStr != "" {
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
		}
	}

	return contents
}

func contentToMessage(content GeminiContent) Message {
	role := content.Role
	if role == "model" {
		role = "assistant"
	}
	message := Message{
		Role:    role,
		Content: content.Parts[0].Text,
	}

	return message
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

func (g *GeminiProvider) Chat(modelName string, messages []Message) (Message, error) {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := GemeniRequest{
		Contents: MessagesToContents(messages),
	}

	if len(messages) > 0 && messages[0].Role == "system" {
		request.SystemInstruction = &GeminiContent{
			Parts: []GeminiPart{
				{
					Text: messages[0].Content.(string),
				},
			},
			Role: "user",
		}
	}

	var response GeminiGenerateContent
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetResult(&response).
		Post(g.baseURL + fmt.Sprintf("/v1beta/%s:generateContent?key=%s", g.DefaultModel(modelName), g.apiKey))

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching response: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching response: %s", res.String())

		}
		return Message{}, fmt.Errorf(msg)
	}

	if response.Candidates[0].FinishReason == "SAFETY" {
		return Message{}, fmt.Errorf("SAFETY")
	}

	return contentToMessage(response.Candidates[0].Content), nil
}

func (g *GeminiProvider) ChatStream(modelName string, messages []Message, callback func(Message) error) error {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := GemeniRequest{
		Contents: MessagesToContents(messages),
	}

	if len(messages) > 0 && messages[0].Role == "system" {
		request.SystemInstruction = &GeminiContent{
			Parts: []GeminiPart{
				{
					Text: messages[0].Content.(string),
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

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching stream response: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching stream response: %s", res.String())
		}
		return fmt.Errorf(msg)
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

		partialMessage := contentToMessage(response.Candidates[0].Content)
		err = callback(partialMessage)
		if err != nil {
			return fmt.Errorf("error in callback: %w", err)
		}

		bufferJSON = ""
	}

	return nil
}

func (g *GeminiProvider) Models() ([]string, error) {
	response, err := g.geminiModels()
	if err != nil {
		return nil, err
	}

	var models []string
	for _, model := range response.Models {
		for _, method := range model.SupportedGenerationMethods {
			if method == "generateContent" {
				models = append(models, model.Name)
			}
		}
	}

	return models, nil
}

func (g *GeminiProvider) geminiModels() (*GeminiModels, error) {
	client := resty.New()

	var response GeminiModels
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetResult(&response).
		Get(g.baseURL + fmt.Sprintf("/v1beta/models?key=%s", g.apiKey))

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching gemini models: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching gemini models: %s", res.String())
		}
		return nil, fmt.Errorf(msg)
	}

	return &response, nil
}
