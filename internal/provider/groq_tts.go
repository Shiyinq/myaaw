package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"teo/internal/config"

	"github.com/go-resty/resty/v2"
)

const (
	groqTTSDefaultModel = "whisper-large-v3-turbo"
	groqTTSAPIEndpoint  = "https://api.groq.com/openai/v1/audio/transcriptions"
)

// GroqTTSProvider implements the TTSProvider interface for Groq.
type GroqTTSProvider struct {
	apiKey       string
	defaultModel string
	client       *resty.Client
}

// GroqTranscriptionResponse represents the JSON response from the Groq transcription API.
type GroqTranscriptionResponse struct {
	Text string `json:"text"`
}

// NewGroqTTSProvider creates a new instance of GroqTTSProvider.
// Note: This function signature needs to match `ttsProviderFactory` if we want to register it directly.
// For now, let's make it return *GroqTTSProvider and we can wrap it if needed during registration.
func NewGroqTTSProvider(apiKey string, defaultModel string) (*GroqTTSProvider, error) {
	if defaultModel == "" {
		defaultModel = groqTTSDefaultModel
	}
	return &GroqTTSProvider{
		apiKey:       apiKey,
		defaultModel: defaultModel,
		client:       resty.New(),
	}, nil
}

// SpeechToText transcribes the given audio file using the Groq API.
func (g *GroqTTSProvider) SpeechToText(audioFile []byte, modelName string) (string, error) {
	model := modelName
	if model == "" {
		model = g.defaultModel
	}

	// Create a buffer to write our multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create the file part
	// Assuming the audio file is ogg, if not, the caller should ensure correct format or we need more info
	part, err := writer.CreateFormFile("file", "audio.ogg")
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	_, err = io.Copy(part, bytes.NewReader(audioFile))
	if err != nil {
		return "", fmt.Errorf("failed to copy audio data to form: %w", err)
	}

	// Add the model field
	err = writer.WriteField("model", model)
	if err != nil {
		return "", fmt.Errorf("failed to write model field: %w", err)
	}

	// Close the writer
	err = writer.Close()
	if err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	resp, err := g.client.R().
		SetAuthToken(g.apiKey).
		SetHeader("Content-Type", writer.FormDataContentType()).
		SetBody(body).
		Post(groqTTSAPIEndpoint)

	if err != nil {
		return "", fmt.Errorf("failed to make request to Groq API: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("Groq API request failed with status %s: %s", resp.Status(), resp.String())
	}

	var transcriptionResponse GroqTranscriptionResponse
	err = json.Unmarshal(resp.Body(), &transcriptionResponse)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal Groq API response: %w", err)
	}

	return transcriptionResponse.Text, nil
}

// Ensure GroqTTSProvider implements TTSProvider
var _ TTSProvider = (*GroqTTSProvider)(nil)

// init registers the Groq TTS provider.
// This function will be called automatically when the package is imported.
func init() {
	// We need a wrapper function to match the ttsProviderFactory signature
	groqFactory := func(apiKey string, defaultModel string) (TTSProvider, error) {
		return NewGroqTTSProvider(apiKey, defaultModel)
	}
	RegisterTTSProvider(config.ProviderGroq, groqFactory)
}
