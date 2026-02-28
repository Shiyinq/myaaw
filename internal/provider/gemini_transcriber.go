package provider

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

type GeminiTranscriber struct {
	client       *genai.Client
	defaultModel string
}

func NewGeminiTranscriber(apiKey string, defaultModel string) Transcriber {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		fmt.Printf("Warning: failed to initialize gemini transcriber client: %v\n", err)
	}

	return &GeminiTranscriber{
		client:       client,
		defaultModel: defaultModel,
	}
}

func (g *GeminiTranscriber) Transcribe(audioFile []byte) (string, error) {
	if g.client == nil {
		return "", fmt.Errorf("gemini client not initialized")
	}

	ctx := context.Background()

	audioPart := &genai.Part{
		InlineData: &genai.Blob{
			MIMEType: "audio/ogg",
			Data:     audioFile,
		},
	}
	textPart := &genai.Part{Text: "Transcribe this audio file exactly as spoken."}

	resp, err := g.client.Models.GenerateContent(ctx, g.defaultModel, []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				textPart,
				audioPart,
			},
		},
	}, nil)

	if err != nil {
		return "", fmt.Errorf("failed to make request to Gemini API: %w", err)
	}

	if resp == nil || len(resp.Candidates) == 0 {
		return "", fmt.Errorf("no candidates returned from Gemini")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("empty transcription received")
	}

	var output []string
	for _, p := range candidate.Content.Parts {
		if p.Text != "" {
			output = append(output, p.Text)
		}
	}

	text := strings.Join(output, "")
	if text == "" {
		return "", fmt.Errorf("empty transcription received or could not extract text")
	}

	return text, nil
}
