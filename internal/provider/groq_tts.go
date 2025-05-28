package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/go-resty/resty/v2"
)

const (
	groqTTSDefaultModel = "whisper-large-v3-turbo"
	groqTTSAPIEndpoint  = "https://api.groq.com/openai/v1/audio/transcriptions"
)

type GroqTTSProvider struct {
	apiKey       string
	defaultModel string
	client       *resty.Client
}

type GroqTranscriptionResponse struct {
	Text string `json:"text"`
}

func NewGroqTTSProvider(apiKey string, defaultModel string) TTSProvider {
	return &GroqTTSProvider{
		apiKey:       apiKey,
		defaultModel: defaultModel,
		client:       resty.New(),
	}
}

func (g *GroqTTSProvider) SpeechToText(audioFile []byte) (string, error) {
	model := g.defaultModel
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "audio.ogg")
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	_, err = io.Copy(part, bytes.NewReader(audioFile))
	if err != nil {
		return "", fmt.Errorf("failed to copy audio data to form: %w", err)
	}

	err = writer.WriteField("model", model)
	if err != nil {
		return "", fmt.Errorf("failed to write model field: %w", err)
	}

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
		return "", fmt.Errorf("groq api request failed with status %s: %s", resp.Status(), resp.String())
	}

	var transcriptionResponse GroqTranscriptionResponse
	err = json.Unmarshal(resp.Body(), &transcriptionResponse)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal Groq API response: %w", err)
	}

	return transcriptionResponse.Text, nil
}
