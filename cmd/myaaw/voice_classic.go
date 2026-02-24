package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"log"
	"myaaw/internal/agent"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/voice"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/spf13/cobra"
)

var (
	vcVideoSource   string
	vcVideoInterval int
	vcSaveFrames    bool
	vcSaveAudio     bool
)

var voiceClassicCmd = &cobra.Command{
	Use:   "voice-classic",
	Short: "Voice chat using STT → Agent → TTS pipeline (supports tools)",
	Long:  "Start a voice conversation that uses speech-to-text, the full agent loop (with tools), and text-to-speech. Works with any LLM provider.",
	Run: func(cmd *cobra.Command, args []string) {
		runVoiceClassic()
	},
}

func init() {
	voiceClassicCmd.Flags().StringVar(&vcVideoSource, "video", "none", "Video source: none, screen, camera, or screen,camera")
	voiceClassicCmd.Flags().IntVar(&vcVideoInterval, "video-interval", 3, "Video frame capture interval in seconds")
	voiceClassicCmd.Flags().BoolVar(&vcSaveFrames, "save-frames", false, "Save captured screen, camera frames to ~/.myaaw/logs/frames/")
	voiceClassicCmd.Flags().BoolVar(&vcSaveAudio, "save-audio", false, "Save user and AI audio per turn to ~/.myaaw/logs/audio/")
}

func runVoiceClassic() {
	homeDir, _ := os.UserHomeDir()

	// Load .env config (needed for LLM provider, transcriber, etc.)
	config.LoadBaseConfig()

	// Create LLM provider and Agent directly (no database needed)
	llmProvider, err := provider.CreateLLMProvider(config.LLMProviderName, config.LLMProviderAPIKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to create LLM provider: %v\n", err)
		os.Exit(1)
	}
	ag := agent.NewAgent(llmProvider)
	modelName := llmProvider.DefaultModel("")

	// Build system prompt
	systemPrompt := agent.NewSystemPromptBuilder(0, "voice-classic").Build()

	// In-memory conversation history
	var conversationHistory []provider.Message
	conversationHistory = append(conversationHistory, provider.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	// Create Synthesizer (TTS) via factory
	synthesizer, err := provider.CreateSynthesizer("gemini", config.LLMProviderAPIKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to create TTS synthesizer: %v\n", err)
		os.Exit(1)
	}

	// Parse video modes
	videoModes := parseVideoModes(vcVideoSource)

	// Setup transcript logger
	transcript, err := newTranscriptLogger()
	if err != nil {
		log.Printf("Warning: could not setup transcript logging: %v", err)
	}
	if transcript != nil {
		defer transcript.Close()
	}

	// Setup audio logging
	var audioDir string
	var turnCounter int
	if vcSaveAudio {
		sessionName := time.Now().Format("2006-01-02") + "/" + time.Now().Format("15.04.05")
		if transcript != nil {
			sessionName = transcript.SessionName
		}
		audioDir = filepath.Join(homeDir, ".myaaw", "logs", "audio", sessionName)
		if err := os.MkdirAll(audioDir, 0755); err != nil {
			log.Printf("Warning: could not create audio dir: %v", err)
			audioDir = ""
		}
	}

	// Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to initialize PortAudio: %v\n", err)
		os.Exit(1)
	}
	defer portaudio.Terminate()

	// Header
	fmt.Println()
	fmt.Println(theme.RenderPrimary("  🎤 MYAAW VOICE CLASSIC 🎤"))
	fmt.Println()
	fmt.Println("  Mode: STT → Agent Loop → TTS")
	fmt.Println()

	// Setup microphone
	mic, err := voice.NewAudioCapture()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to open microphone: %v\n", err)
		os.Exit(1)
	}
	defer mic.Close()

	// Setup speaker
	speaker, err := voice.NewAudioPlayer(24000) // Gemini TTS outputs 24kHz
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to open speaker: %v\n", err)
		os.Exit(1)
	}
	defer speaker.Close()

	if err := mic.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to start microphone: %v\n", err)
		os.Exit(1)
	}
	if err := speaker.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to start speaker: %v\n", err)
		os.Exit(1)
	}

	// Status display
	fmt.Print(theme.RenderSecondary("  🎤 Mic: "))
	fmt.Println("Ready")
	fmt.Print(theme.RenderSecondary("  🔊 Speaker: "))
	fmt.Println("Ready")

	for _, mode := range videoModes {
		switch mode {
		case voice.VideoModeScreen:
			fmt.Print(theme.RenderSecondary("  🖥️  Screen: "))
			fmt.Printf("Capturing every %ds\n", vcVideoInterval)
		case voice.VideoModeCamera:
			fmt.Print(theme.RenderSecondary("  📷 Camera: "))
			fmt.Printf("Capturing every %ds\n", vcVideoInterval)
		}
	}

	if transcript != nil {
		fmt.Print(theme.RenderSecondary("  📝 Transcript: "))
		fmt.Printf("voice-classic-transcript-%s.log\n", time.Now().Format("2006-01-02"))
	}

	// Setup frame saving
	var framesDir string
	if vcSaveFrames && len(videoModes) > 0 {
		sessionName := time.Now().Format("2006-01-02") + "/" + time.Now().Format("15.04.05")
		if transcript != nil {
			sessionName = transcript.SessionName
		}
		framesDir = filepath.Join(homeDir, ".myaaw", "logs", "frames", sessionName)
		if err := os.MkdirAll(framesDir, 0755); err != nil {
			log.Printf("Warning: could not create frames dir: %v", err)
			framesDir = ""
		} else {
			fmt.Print(theme.RenderSecondary("  💾 Frames: "))
			fmt.Printf("~/.myaaw/logs/frames/%s/\n", sessionName)
		}
	}

	if audioDir != "" {
		fmt.Print(theme.RenderSecondary("  🎵 Audio: "))
		fmt.Printf("~/.myaaw/logs/audio/%s/\n", time.Now().Format("2006-01-02")+"/"+time.Now().Format("15.04.05"))
	}

	fmt.Println()
	fmt.Println(theme.RenderPrimary("  Start speaking! Press Ctrl+C to stop."))
	fmt.Println()

	// Create VAD (Voice Activity Detection)
	vad := voice.NewVAD(800, 1500*time.Millisecond)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	// Start video capture if enabled
	var latestFrame []byte
	var frameMu sync.Mutex
	var frameCounter int

	if len(videoModes) > 0 {
		capturer := voice.NewVideoCapturer(videoModes, time.Duration(vcVideoInterval)*time.Second)
		frameChan := make(chan []byte, 5)

		wg.Add(1)
		go func() {
			defer wg.Done()
			capturer.Run(ctx, frameChan)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case frame := <-frameChan:
					frameMu.Lock()
					latestFrame = frame
					frameMu.Unlock()

					// Save frame if enabled
					if framesDir != "" {
						frameCounter++
						framePath := filepath.Join(framesDir, fmt.Sprintf("frame-%04d.jpg", frameCounter))
						os.WriteFile(framePath, frame, 0644)
					}
				}
			}
		}()
	}

	// Main conversation loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		var audioBuffer []byte

		fmt.Print("  🎤 Listening... ")

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Read audio chunk from mic
			chunk, err := mic.Read()
			if err != nil {
				log.Printf("Mic read error: %v", err)
				continue
			}

			event := vad.Process(chunk)

			switch event {
			case voice.VADSpeechStarted:
				audioBuffer = make([]byte, 0, voice.InputSampleRate*2*5) // pre-alloc ~5s
				audioBuffer = append(audioBuffer, chunk...)

			case voice.VADSpeechActive:
				if audioBuffer != nil {
					audioBuffer = append(audioBuffer, chunk...)
				}

			case voice.VADSpeechEnded:
				if audioBuffer == nil || len(audioBuffer) < voice.InputSampleRate {
					// Too short, ignore
					audioBuffer = nil
					continue
				}

				fmt.Print("\r\033[K")
				fmt.Println("  ⏳ Processing...")

				// Save user audio if enabled
				turnCounter++
				if audioDir != "" {
					userAudioPath := filepath.Join(audioDir, fmt.Sprintf("turn-%03d-user.wav", turnCounter))
					saveWAV(userAudioPath, audioBuffer, voice.InputSampleRate)
				}

				// 1. STT: Convert audio to text
				wavData := pcmToWAV(audioBuffer, voice.InputSampleRate)
				transcribedText, err := transcribeAudio(wavData)
				if err != nil {
					log.Printf("STT error: %v", err)
					fmt.Println("  ❌ Could not transcribe audio")
					fmt.Print("  🎤 Listening... ")
					audioBuffer = nil
					continue
				}

				if strings.TrimSpace(transcribedText) == "" {
					fmt.Print("  🎤 Listening... ")
					audioBuffer = nil
					continue
				}

				fmt.Printf("  🎤 You: %s\n", transcribedText)
				if transcript != nil {
					transcript.LogUser(transcribedText)
				}

				// 2. Agent Loop: Send to Agent directly
				fmt.Println("  ⏳ AI is thinking...")

				// Build user message with optional image
				userContent := transcribedText
				frameMu.Lock()
				currentFrame := latestFrame
				frameMu.Unlock()

				userMsg := provider.Message{
					Role:    "user",
					Content: userContent,
				}
				if currentFrame != nil {
					encoded := base64.StdEncoding.EncodeToString(currentFrame)
					userMsg.Images = []string{encoded}
				}

				conversationHistory = append(conversationHistory, userMsg)

				response, err := ag.Run(modelName, conversationHistory)
				if err != nil {
					log.Printf("Agent error: %v", err)
					fmt.Println("  ❌ Agent error")
					fmt.Print("  🎤 Listening... ")
					audioBuffer = nil
					continue
				}

				responseText := ""
				if str, ok := response.Content.(string); ok {
					responseText = str
				}

				// Add assistant response to history
				conversationHistory = append(conversationHistory, response)
				fmt.Printf("\n  🐱 Myaaw: %s\n\n", responseText)
				if transcript != nil {
					transcript.LogAI(responseText)
				}

				// 3. TTS: Convert response to audio
				fmt.Print("  ⏳ Generating speech...")

				// Stop mic during TTS to prevent buffer overflow
				mic.Stop()

				audioData, _, err := synthesizer.Synthesize(ctx, responseText)
				if err != nil {
					log.Printf("TTS error: %v", err)
					fmt.Print("\r\033[K")
					fmt.Println("  ❌ Could not synthesize speech")
				} else {
					// Save AI audio if enabled
					if audioDir != "" {
						aiAudioPath := filepath.Join(audioDir, fmt.Sprintf("turn-%03d-ai.wav", turnCounter))
						saveWAV(aiAudioPath, audioData, 24000)
					}

					fmt.Print("\r\033[K")
					fmt.Println("  🔊 Speaking...")

					// Play audio
					speaker.Play(audioData)

					// Wait for playback to finish
					for speaker.IsPlaying() {
						time.Sleep(100 * time.Millisecond)
					}
				}

				// Restart mic and reset VAD
				mic.Start()
				vad.Reset()

				fmt.Print("  🎤 Listening... ")
				audioBuffer = nil

			case voice.VADSilence:
				// Do nothing, keep listening
			}
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\n\n  👋 Goodbye!")
	cancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		log.Println("Force shutdown after timeout")
	}
}

// transcribeAudio sends audio to the Gemini transcriber for STT
func transcribeAudio(wavData []byte) (string, error) {
	transcriber, err := provider.CreateTranscriber(config.TranscriberProviderName, config.TranscriberAPIKey)
	if err != nil {
		return "", fmt.Errorf("failed to create transcriber: %w", err)
	}
	return transcriber.Transcribe(wavData)
}

// pcmToWAV wraps raw PCM data in a WAV header
func pcmToWAV(pcmData []byte, sampleRate int) []byte {
	var buf bytes.Buffer

	dataSize := len(pcmData)
	fileSize := 36 + dataSize

	// RIFF header
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, int32(fileSize))
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, int32(16)) // chunk size
	binary.Write(&buf, binary.LittleEndian, int16(1))  // PCM format
	binary.Write(&buf, binary.LittleEndian, int16(1))  // mono
	binary.Write(&buf, binary.LittleEndian, int32(sampleRate))
	binary.Write(&buf, binary.LittleEndian, int32(sampleRate*2)) // byte rate
	binary.Write(&buf, binary.LittleEndian, int16(2))            // block align
	binary.Write(&buf, binary.LittleEndian, int16(16))           // bits per sample

	// data chunk
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, int32(dataSize))
	buf.Write(pcmData)

	return buf.Bytes()
}

// saveWAV saves PCM audio data as a WAV file
func saveWAV(path string, pcmData []byte, sampleRate int) {
	wavData := pcmToWAV(pcmData, sampleRate)
	if err := os.WriteFile(path, wavData, 0644); err != nil {
		log.Printf("Warning: could not save audio to %s: %v", path, err)
	}
}

// parseVCVideoModes parses video modes (reuse logic from voice.go)
func parseVCVideoModes(source string) []voice.VideoMode {
	if source == "none" || source == "" {
		return nil
	}

	var modes []voice.VideoMode
	parts := strings.Split(source, ",")
	for _, p := range parts {
		switch strings.TrimSpace(p) {
		case "screen":
			modes = append(modes, voice.VideoModeScreen)
		case "camera":
			modes = append(modes, voice.VideoModeCamera)
		}
	}
	return modes
}
