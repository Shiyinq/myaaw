package provider

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// GeminiSynthesizer converts text to speech using Gemini's GenerateContent API
type GeminiSynthesizer struct {
	client *genai.Client
	model  string
	voice  string
}

// NewGeminiSynthesizer creates a new TTS synthesizer using Gemini
func NewGeminiSynthesizer(apiKey string, defaultModel string) (Synthesizer, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	return &GeminiSynthesizer{
		client: client,
		model:  defaultModel,
		voice:  "Kore",
	}, nil
}

// Synthesize converts text to PCM audio bytes using Gemini TTS
// Returns raw audio data and its MIME type
func (s *GeminiSynthesizer) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	config := &genai.GenerateContentConfig{
		ResponseModalities: []string{"AUDIO"},
		SpeechConfig: &genai.SpeechConfig{
			VoiceConfig: &genai.VoiceConfig{
				PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
					VoiceName: s.voice,
				},
			},
		},
	}

	result, err := s.client.Models.GenerateContent(ctx, s.model, genai.Text(text), config)
	if err != nil {
		return nil, "", fmt.Errorf("gemini TTS request failed: %w", err)
	}

	if result == nil || len(result.Candidates) == 0 {
		return nil, "", fmt.Errorf("no candidates returned from Gemini TTS")
	}

	candidate := result.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil, "", fmt.Errorf("empty response from Gemini TTS")
	}

	// Extract audio data from InlineData
	for _, part := range candidate.Content.Parts {
		if part.InlineData != nil && len(part.InlineData.Data) > 0 {
			return part.InlineData.Data, part.InlineData.MIMEType, nil
		}
	}

	return nil, "", fmt.Errorf("no audio data in Gemini TTS response")
}

// SetVoice changes the TTS voice
func (s *GeminiSynthesizer) SetVoice(voiceName string) {
	s.voice = voiceName
}

// SetModel changes the TTS model
func (s *GeminiSynthesizer) SetModel(model string) {
	s.model = model
}
