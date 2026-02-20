package provider

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

type GeminiTranscriber struct {
	apiKey       string
	defaultModel string
	client       *resty.Client
}

func NewGeminiTranscriber(apiKey string, defaultModel string) Transcriber {
	return &GeminiTranscriber{
		apiKey:       apiKey,
		defaultModel: defaultModel,
		client:       resty.New().SetTimeout(120 * time.Second),
	}
}

func (g *GeminiTranscriber) Transcribe(audioFile []byte) (string, error) {
	// Encode audio to base64
	encodedAudio := base64.StdEncoding.EncodeToString(audioFile)

	// Construct request body for Gemini
	// We use the same structure as in gemini.go but simplified for this specific use case
	requestBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{
						"text": "Transcribe this audio file exactly as spoken.",
					},
					{
						"inline_data": map[string]interface{}{
							"mime_type": "audio/ogg",
							"data":      encodedAudio,
						},
					},
				},
			},
		},
	}

	var response GeminiGenerateContent
	resp, err := g.client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(requestBody).
		SetResult(&response).
		Post(fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/%s:generateContent?key=%s", g.defaultModel, g.apiKey))

	if err != nil {
		return "", fmt.Errorf("failed to make request to Gemini API: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("gemini api request failed with status %s: %s", resp.Status(), resp.String())
	}

	if len(response.Candidates) == 0 {
		return "", fmt.Errorf("no candidates returned from Gemini")
	}

	// Extract text from the first candidate
	if len(response.Candidates[0].Content.Parts) > 0 {
		return response.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("empty transcription received")
}
