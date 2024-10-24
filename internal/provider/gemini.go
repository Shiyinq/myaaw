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

type InlineData struct {
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"`
}

type Part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inline_data,omitempty"`
}

type Content struct {
	Parts []Part `json:"parts,omitempty"`
	Role  string `json:"role,omitempty"`
}

type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type Candidate struct {
	Content       Content        `json:"content"`
	FinishReason  string         `json:"finishReason"`
	Index         int            `json:"index"`
	SafetyRatings []SafetyRating `json:"safetyRatings"`
}

type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type GenerateContent struct {
	Candidates    []Candidate   `json:"candidates"`
	UsageMetadata UsageMetadata `json:"usageMetadata"`
}

type GemeniRequest struct {
	Contents          []Content `json:"contents"`
	SystemInstruction *Content  `json:"systemInstruction,omitempty"`
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
	baseURL string
	apiKey  string
}

func NewGeminiProvider(baseURL string, apiKey string) LLMProvider {
	return &GeminiProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

func MessagesToContents(messages []Message) []Content {
	var contents []Content
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

		var content Content
		if contentStr != "" {
			content = Content{
				Parts: []Part{
					{
						Text: contentStr,
					},
				},
				Role: role,
			}

			if message.Images != nil {
				image := &InlineData{
					MimeType: "image/jpeg",
					Data:     message.Images[0],
				}
				content.Parts = append(content.Parts, Part{InlineData: image})
			}

			contents = append(contents, content)
		}
	}

	return contents
}

func ContentToMessage(content Content) Message {
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

func (o *GeminiProvider) ProviderName() string {
	return "gemini"
}

func (o *GeminiProvider) Chat(modelName string, messages []Message) (Message, error) {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := GemeniRequest{
		Contents: MessagesToContents(messages),
	}

	if len(messages) > 0 && messages[0].Role == "system" {
		request.SystemInstruction = &Content{
			Parts: []Part{
				{
					Text: messages[0].Content.(string),
				},
			},
			Role: "user",
		}
	}

	var response GenerateContent
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetResult(&response).
		Post(o.baseURL + fmt.Sprintf("/v1beta/%s:generateContent?key=%s", modelName, o.apiKey))

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

	return ContentToMessage(response.Candidates[0].Content), nil
}

func (o *GeminiProvider) ChatStream(modelName string, messages []Message, callback func(Message) error) error {
	client := resty.New()
	client.SetTimeout(120 * time.Second)

	request := GemeniRequest{
		Contents: MessagesToContents(messages),
	}

	if len(messages) > 0 && messages[0].Role == "system" {
		request.SystemInstruction = &Content{
			Parts: []Part{
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
		Post(o.baseURL + fmt.Sprintf("/v1beta/%s:streamGenerateContent?key=%s", modelName, o.apiKey))

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching stream response: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching stream response: %s", res.String())
		}
		return fmt.Errorf(msg)
	}

	defer res.RawBody().Close()

	reader := bufio.NewReader(res.RawBody())
	var response GenerateContent
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

		partialMessage := ContentToMessage(response.Candidates[0].Content)
		err = callback(partialMessage)
		if err != nil {
			return fmt.Errorf("error in callback: %w", err)
		}

		bufferJSON = ""
	}

	return nil
}

func (o *GeminiProvider) Models() ([]string, error) {
	response, err := o.geminiModels()
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

func (o *GeminiProvider) geminiModels() (*GeminiModels, error) {
	client := resty.New()

	var response GeminiModels
	res, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetResult(&response).
		Get(o.baseURL + fmt.Sprintf("/v1beta/models?key=%s", o.apiKey))

	if err != nil || res.StatusCode() != 200 {
		msg := fmt.Sprintf("error fetching gemini models: %v", err)
		if err == nil {
			msg = fmt.Sprintf("error fetching gemini models: %s", res.String())
		}
		return nil, fmt.Errorf(msg)
	}

	return &response, nil
}
