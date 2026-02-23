package provider

import (
	"context"
	"fmt"
	"log"
	"sync"

	"google.golang.org/genai"
)

const (
	LiveModel = "gemini-2.5-flash-native-audio-preview-12-2025"
)

// LiveResponseType indicates what kind of event this is
type LiveResponseType int

const (
	LiveResponseAudio       LiveResponseType = iota // audio PCM data
	LiveResponseText                                // model text output
	LiveResponseTurnDone                            // model finished speaking
	LiveResponseUserSpeech                          // user speech transcription (input)
	LiveResponseModelSpeech                         // model speech transcription (output)
)

// LiveResponse represents a response from the Gemini Live API
type LiveResponse struct {
	Type      LiveResponseType
	AudioData []byte // raw PCM audio data (24kHz, 16-bit, mono)
	Text      string // text content
}

// GeminiLiveSession manages a real-time bidirectional session with Gemini Live API
type GeminiLiveSession struct {
	client  *genai.Client
	session *genai.Session
	ctx     context.Context
	cancel  context.CancelFunc
	sendMu  sync.Mutex // protects concurrent WebSocket writes
}

// NewGeminiLiveSession creates and connects a new Gemini Live session
func NewGeminiLiveSession(apiKey string, systemInstruction string) (*GeminiLiveSession, error) {
	ctx, cancel := context.WithCancel(context.Background())

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	config := &genai.LiveConnectConfig{
		ResponseModalities: []genai.Modality{genai.ModalityAudio},
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: systemInstruction},
			},
		},
		SpeechConfig: &genai.SpeechConfig{
			VoiceConfig: &genai.VoiceConfig{
				PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
					VoiceName: "Aoede",
				},
			},
		},
		// Enable transcription for both input and output audio
		InputAudioTranscription:  &genai.AudioTranscriptionConfig{},
		OutputAudioTranscription: &genai.AudioTranscriptionConfig{},
	}

	session, err := client.Live.Connect(ctx, LiveModel, config)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to connect to Gemini Live API: %w", err)
	}

	return &GeminiLiveSession{
		client:  client,
		session: session,
		ctx:     ctx,
		cancel:  cancel,
	}, nil
}

// SendAudio sends raw PCM audio data to the session (16kHz, 16-bit, mono)
func (s *GeminiLiveSession) SendAudio(data []byte) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.session.SendRealtimeInput(
		genai.LiveRealtimeInput{
			Audio: &genai.Blob{
				MIMEType: "audio/pcm;rate=16000",
				Data:     data,
			},
		},
	)
}

// SendVideoFrame sends a JPEG image frame to the session
func (s *GeminiLiveSession) SendVideoFrame(jpegData []byte) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.session.SendRealtimeInput(
		genai.LiveRealtimeInput{
			Video: &genai.Blob{
				MIMEType: "image/jpeg",
				Data:     jpegData,
			},
		},
	)
}

// Receive listens for responses from the session and sends them to the provided channel.
// This function blocks and should be run in a goroutine.
func (s *GeminiLiveSession) Receive(responseChan chan<- LiveResponse) {
	defer close(responseChan)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			msg, err := s.session.Receive()
			if err != nil {
				if s.ctx.Err() != nil {
					return // context cancelled, normal shutdown
				}
				log.Printf("Error receiving from Live API: %v", err)
				return
			}

			if msg == nil {
				continue
			}

			// Handle server content (audio, text, transcription)
			if msg.ServerContent != nil {
				// Handle input transcription (what user said)
				if msg.ServerContent.InputTranscription != nil && msg.ServerContent.InputTranscription.Text != "" {
					responseChan <- LiveResponse{
						Type: LiveResponseUserSpeech,
						Text: msg.ServerContent.InputTranscription.Text,
					}
				}

				// Handle output transcription (what model said, as text)
				if msg.ServerContent.OutputTranscription != nil && msg.ServerContent.OutputTranscription.Text != "" {
					responseChan <- LiveResponse{
						Type: LiveResponseModelSpeech,
						Text: msg.ServerContent.OutputTranscription.Text,
					}
				}

				// Handle model audio/text content
				if msg.ServerContent.ModelTurn != nil {
					for _, part := range msg.ServerContent.ModelTurn.Parts {
						if part.InlineData != nil && part.InlineData.Data != nil {
							responseChan <- LiveResponse{
								Type:      LiveResponseAudio,
								AudioData: part.InlineData.Data,
							}
						}
						if part.Text != "" {
							responseChan <- LiveResponse{
								Type: LiveResponseText,
								Text: part.Text,
							}
						}
					}
				}

				if msg.ServerContent.TurnComplete {
					responseChan <- LiveResponse{Type: LiveResponseTurnDone}
				}
			}
		}
	}
}

// Close gracefully closes the session
func (s *GeminiLiveSession) Close() {
	s.cancel()
	if s.session != nil {
		if err := s.session.Close(); err != nil {
			log.Printf("Error closing Live session: %v", err)
		}
	}
}
