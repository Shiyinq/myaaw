package service_test

import (
	"errors"
	"testing"
	"teo/internal/config"
	"teo/internal/pkg"
	"teo/internal/provider"
	botService "teo/internal/services/bot/service" // Alias to avoid conflict
)

// mockTTSProvider implements the provider.TTSProvider interface for testing.
type mockTTSProvider struct {
	TranscribeFunc func(audioFile []byte, modelName string) (string, error)
}

func (m *mockTTSProvider) SpeechToText(audioFile []byte, modelName string) (string, error) {
	if m.TranscribeFunc != nil {
		return m.TranscribeFunc(audioFile, modelName)
	}
	return "mocked transcription", nil
}

func TestAudioMessage_CreateMessage_Success(t *testing.T) {
	// Setup mock TTS provider
	mockProvider := &mockTTSProvider{}

	// Instantiate AudioMessage factory
	audioMessageFactory := botService.NewAudioMessage(mockProvider)

	// Setup incoming chat with Voice data
	chat := &pkg.TelegramIncommingChat{
		Message: pkg.UserMessage{
			Voice: &pkg.Voice{
				FileID: "fake_voice_file_id",
			},
		},
	}

	// TODO: Mock pkg.GetFilePath and pkg.DownloadTgFile to return success
	// This is crucial for the test to proceed without actual file operations.
	// For example, by using a monkey patching library or by refactoring
	// pkg.GetFilePath and pkg.DownloadTgFile to be interface methods that can be mocked.
	// For now, we assume they would work or this test focuses on the transcription part
	// after a successful download (which is hard to do without deeper mocking).

	// Call CreateMessage
	providerMessage := audioMessageFactory.CreateMessage(chat)

	// TODO: Assert that the returned provider.Message has Role="user" and Content="mocked transcription".
	// Example:
	// if providerMessage.Role != "user" {
	//  t.Errorf("Expected Role to be 'user', got %s", providerMessage.Role)
	// }
	// if providerMessage.Content != "mocked transcription" {
	//  t.Errorf("Expected Content to be 'mocked transcription', got %s", providerMessage.Content)
	// }
	_ = providerMessage // Placeholder to use providerMessage
}

func TestAudioMessage_CreateMessage_DownloadError(t *testing.T) {
	// TODO: Mock pkg.GetFilePath and/or pkg.DownloadTgFile to return an error.
	// Verify that CreateMessage handles this (e.g., returns a message with error content).
	// This requires the ability to mock these package-level functions.
	// Example (conceptual, if pkg.GetFilePath could be mocked):
	// originalGetFilePath := pkg.GetFilePath
	// pkg.GetFilePath = func(fileID string) (string, error) {
	//  return "", errors.New("mocked download error")
	// }
	// defer func() { pkg.GetFilePath = originalGetFilePath }() // Restore
	//
	// mockProvider := &mockTTSProvider{}
	// audioMessageFactory := botService.NewAudioMessage(mockProvider)
	// chat := &pkg.TelegramIncommingChat{ /* ... */ }
	// providerMessage := audioMessageFactory.CreateMessage(chat)
	// // Assert providerMessage.Content contains error indication
}

func TestAudioMessage_CreateMessage_TranscriptionError(t *testing.T) {
	// Configure mock TTSProvider to return an error
	mockProvider := &mockTTSProvider{
		TranscribeFunc: func(audioFile []byte, modelName string) (string, error) {
			return "", errors.New("mocked transcription error")
		},
	}

	audioMessageFactory := botService.NewAudioMessage(mockProvider)

	chat := &pkg.TelegramIncommingChat{
		Message: pkg.UserMessage{
			Voice: &pkg.Voice{
				FileID: "fake_voice_file_id_for_transcription_error",
			},
		},
	}

	// TODO: Mock pkg.GetFilePath and pkg.DownloadTgFile to return success, similar to the success test.
	// Or ensure test setup bypasses actual download if focusing only on transcription error handling.

	providerMessage := audioMessageFactory.CreateMessage(chat)

	// TODO: Verify that CreateMessage handles this (e.g., returns a message with error content indicating transcription failure).
	// Example:
	// if providerMessage.Content != "[Error transcribing audio]" { // Or similar, based on actual error message
	//  t.Errorf("Expected error content, got %s", providerMessage.Content)
	// }
	_ = providerMessage // Placeholder
}

func TestNewMessage_AudioRouting(t *testing.T) {
	// Setup mock TTS provider
	mockTTS := &mockTTSProvider{
		TranscribeFunc: func(audioFile []byte, modelName string) (string, error) {
			// For this test, assume transcription is successful and returns this specific text.
			// Also assume that pkg.GetFilePath and pkg.DownloadTgFile would be successfully mocked
			// in a full test implementation, so AudioMessage.CreateMessage reaches the transcription step.
			return "mocked transcription for audio routing test", nil
		},
	}

	// Create a TelegramIncommingChat with Voice data
	chat := &pkg.TelegramIncommingChat{
		Message: pkg.UserMessage{
			Voice: &pkg.Voice{
				FileID: "fake_audio_file_id_for_routing",
			},
			// Text should be empty for a pure voice message
			Text: "",
		},
	}

	llmName := config.ProviderGroq // Example LLM provider name

	// Call the updated NewMessage function
	// NewMessage(chat *pkg.TelegramIncommingChat, llmProviderName string, ttsProvider provider.TTSProvider)
	returnedMessage := botService.NewMessage(chat, llmName, mockTTS)

	// TODO: Assert that returnedMessage.Content is the result of the mock transcription.
	// This indirectly verifies that the AudioMessage factory was used.
	// This assertion depends on the successful mocking of file download operations
	// within the AudioMessage.CreateMessage method, which is currently a TODO there.
	// If file operations were mocked to succeed, then:
	// if returnedMessage.Content != "mocked transcription for audio routing test" {
	//  t.Errorf("Expected content from mock TTS, got '%s'", returnedMessage.Content)
	// }
	// if returnedMessage.Role != "user" {
	//  t.Errorf("Expected Role to be 'user', got '%s'", returnedMessage.Role)
	// }

	// Test the case where TTS provider is nil (e.g., not configured or failed to initialize)
	returnedMessageNilTTS := botService.NewMessage(chat, llmName, nil)

	// TODO: Assert that returnedMessageNilTTS.Content indicates transcription is unavailable.
	// This checks the nil ttsProvider branch in NewMessage.
	// expectedContent := "[Audio transcription not available]"
	// if returnedMessageNilTTS.Content != expectedContent {
	//  t.Errorf("Expected content '%s' for nil TTS, got '%s'", expectedContent, returnedMessageNilTTS.Content)
	// }

	_ = returnedMessage         // Placeholder to use the variable
	_ = returnedMessageNilTTS   // Placeholder to use the variable
	_ = provider.ErrProviderNotFound // Placeholder to use provider import (if still needed, else remove)
}
