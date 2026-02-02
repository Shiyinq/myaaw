package provider

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"teo/internal/tools"
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

type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
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
			geminiPart, ok := message.Content.(GeminiPart)
			if !ok {
				log.Println("unknown type content")
			}

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
	}

	return contents
}

func contentToMessage(content GeminiContent) Message {
	role := content.Role
	if role == "model" {
		role = "assistant"
	}

	// Guard against empty Parts
	if len(content.Parts) == 0 {
		return Message{
			Role:    role,
			Content: "",
		}
	}

	if content.Parts[0].FunctionCall != nil {
		return Message{
			Role:      role,
			Content:   "",
			ToolCalls: []ToolCall{},
		}
	} else {
		return Message{
			Role:    role,
			Content: content.Parts[0].Text,
		}
	}
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

func (g *GeminiProvider) hasFunctionCall(response GeminiGenerateContent) bool {
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				return true
			}
		}
	}
	return false
}

func (g *GeminiProvider) geminiContentsToMessages(contents []GeminiContent) []Message {
	var messages []Message

	for _, content := range contents {
		role := content.Role
		if role == "model" {
			role = "assistant"
		}

		message := Message{
			Role: role,
		}

		for _, part := range content.Parts {
			if part.FunctionCall != nil {
				// ReAct: This is an Action
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				message.Action = part.FunctionCall.Name
				message.ActionInput = string(argsJSON)
				message.Thought = fmt.Sprintf("Using tool: %s", part.FunctionCall.Name)
				message.Content = part
			} else if part.FunctionResponse != nil {
				// ReAct: This is an Observation
				role = "tool"
				message.Role = role
				message.Name = part.FunctionResponse.Name
				if resp, ok := part.FunctionResponse.Response["response"].(string); ok {
					message.Observation = resp
					message.Content = resp
				} else {
					obsJSON, _ := json.Marshal(part.FunctionResponse.Response)
					message.Observation = string(obsJSON)
					message.Content = string(obsJSON)
				}
			} else {
				// Normal text response
				message.Content = part.Text
			}
		}

		messages = append(messages, message)
	}

	return messages
}

func (g *GeminiProvider) geminiToolCalls(messages []GeminiContent, parts []GeminiPart, originalMessages []Message) []Message {
	for _, part := range parts {
		if part.FunctionCall != nil {
			functionName := part.FunctionCall.Name
			functionArgs := part.FunctionCall.Args
			argsJSON, err := json.Marshal(functionArgs)
			if err != nil {
				fmt.Println("Error marshaling functionArgs:", err)
				continue
			}

			// Log ReAct Trace using shared functions
			thought := GenerateThought(functionName)
			LogThought(thought)
			LogAction(functionName, string(argsJSON))

			tool := tools.NewTools(functionName, string(argsJSON))

			LogObservation(tool)

			responseTool := []GeminiContent{
				{
					Role:  "model",
					Parts: parts,
				},
				{
					Role: "user",
					Parts: []GeminiPart{
						{
							FunctionResponse: &GeminiFunctionResponse{
								Name: functionName,
								Response: map[string]interface{}{
									"response": tool,
								},
							},
						},
					},
				},
				// Prompt LLM to think about the observation and plan next step
				{
					Role: "user",
					Parts: []GeminiPart{
						{
							Text: ThoughtPrompt,
						},
					},
				},
			}
			messages = append(messages, responseTool...)
		}
	}

	// Convert back to Messages and prepend system prompt if it exists
	result := g.geminiContentsToMessages(messages)

	// Prepend system prompt from original messages
	if len(originalMessages) > 0 && originalMessages[0].Role == "system" {
		result = append([]Message{originalMessages[0]}, result...)
	}

	return result
}

func (g *GeminiProvider) Chat(modelName string, messages []Message) (Message, error) {
	return g.chatWithIteration(modelName, messages, 0)
}

func (g *GeminiProvider) chatWithIteration(modelName string, messages []Message, iteration int) (Message, error) {
	// Check max iterations using shared function
	if IsMaxIterationsReached(iteration, DefaultReactConfig) {
		return Message{
			Role:    "assistant",
			Content: MaxIterationsMessage,
		}, nil
	}

	LogIteration(iteration, DefaultReactConfig, false)

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
		// Add step awareness warning using shared function
		systemText += GetStepAwarenessWarning(iteration, DefaultReactConfig)

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

	if g.hasFunctionCall(response) {
		respTool := g.geminiToolCalls(MessagesToContents(messages), response.Candidates[0].Content.Parts, messages)
		return g.chatWithIteration(modelName, respTool, iteration+1)
	}

	if response.Candidates[0].FinishReason == "SAFETY" {
		return Message{}, fmt.Errorf("SAFETY")
	}

	return contentToMessage(response.Candidates[0].Content), nil
}

func (g *GeminiProvider) ChatStream(modelName string, messages []Message, callback func(Message) error) error {
	return g.chatStreamWithIteration(modelName, messages, callback, 0)
}

func (g *GeminiProvider) chatStreamWithIteration(modelName string, messages []Message, callback func(Message) error, iteration int) error {
	// Check max iterations using shared function
	if IsMaxIterationsReached(iteration, DefaultReactConfig) {
		return callback(Message{
			Role:    "assistant",
			Content: MaxIterationsMessage,
		})
	}

	LogIteration(iteration, DefaultReactConfig, true)

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

		// Add step awareness warning using shared function
		systemText += GetStepAwarenessWarning(iteration, DefaultReactConfig)

		request.SystemInstruction = &GeminiContent{
			Parts: []GeminiPart{
				{
					Text: systemText,
				},
			},
			Role: "user",
		}
	}

	res, _ := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(request).
		SetDoNotParseResponse(true).
		Post(g.baseURL + fmt.Sprintf("/v1beta/%s:streamGenerateContent?key=%s", g.DefaultModel(modelName), g.apiKey))

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

		partialMessage := contentToMessage(response.Candidates[0].Content)
		err = callback(partialMessage)
		if err != nil {
			return fmt.Errorf("error in callback: %w", err)
		}

		bufferJSON = ""

		if g.hasFunctionCall(response) {
			respTool := g.geminiToolCalls(MessagesToContents(messages), response.Candidates[0].Content.Parts, messages)
			return g.chatStreamWithIteration(modelName, respTool, callback, iteration+1)
		}
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
