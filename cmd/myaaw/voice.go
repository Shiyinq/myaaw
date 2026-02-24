package main

import (
	"context"
	"fmt"
	"log"
	"math"
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
	videoSource   string
	videoInterval int
	saveFrames    bool
)

var voiceCmd = &cobra.Command{
	Use:   "voice",
	Short: "Talk to your AI assistant with real-time voice (Gemini Live)",
	Long:  "Start a real-time voice conversation with live audio, screen sharing, and camera support.",
	Run: func(cmd *cobra.Command, args []string) {
		config.LoadBaseConfig()
		runVoice()
	},
}

func init() {
	voiceCmd.Flags().StringVar(&videoSource, "video", "none", "Video source: none, screen, camera, or screen,camera")
	voiceCmd.Flags().IntVar(&videoInterval, "video-interval", 3, "Video frame capture interval in seconds")
	voiceCmd.Flags().BoolVar(&saveFrames, "save-frames", false, "Save captured screen, camera frames to ~/.myaaw/logs/frames/")
}

// voiceTranscriptLogger handles writing conversation transcripts to a log file
type voiceTranscriptLogger struct {
	file        *os.File
	SessionName string // e.g. "voice-transcript-2026-02-24"
}

func newTranscriptLogger() (*voiceTranscriptLogger, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	logDir := filepath.Join(homeDir, ".myaaw", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	timestamp := time.Now().Format("2006-01-02")
	sessionTime := time.Now().Format("15.04.05")
	sessionName := fmt.Sprintf("%s/%s", timestamp, sessionTime)
	logPath := filepath.Join(logDir, fmt.Sprintf("voice-transcript-%s.log", timestamp))

	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}

	// Write session header
	fmt.Fprintf(f, "\n--- Voice Session %s ---\n", time.Now().Format("2006-01-02 15:04:05"))
	return &voiceTranscriptLogger{file: f, SessionName: sessionName}, nil
}

func (l *voiceTranscriptLogger) LogUser(text string) {
	if l.file != nil {
		fmt.Fprintf(l.file, "[%s] 🎤 User: %s\n", time.Now().Format("15:04:05"), text)
	}
}

func (l *voiceTranscriptLogger) LogAI(text string) {
	if l.file != nil {
		fmt.Fprintf(l.file, "[%s] 🐱 Myaaw: %s\n", time.Now().Format("15:04:05"), text)
	}
}

func (l *voiceTranscriptLogger) Close() {
	if l.file != nil {
		fmt.Fprintf(l.file, "--- Session Ended %s ---\n\n", time.Now().Format("2006-01-02 15:04:05"))
		l.file.Close()
	}
}

func runVoice() {
	// Setup logging
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Error getting home directory:", err)
	}
	logDir := filepath.Join(homeDir, ".myaaw", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatal("Error creating log directory:", err)
	}
	logPath := filepath.Join(logDir, "myaaw-voice.log")
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

	// Setup transcript logger
	transcript, err := newTranscriptLogger()
	if err != nil {
		log.Printf("Warning: could not create transcript logger: %v", err)
	}
	if transcript != nil {
		defer transcript.Close()
	}

	// Parse video modes
	videoModes := parseVideoModes(videoSource)

	// Print header
	fmt.Println()
	fmt.Println(theme.RenderPrimary(" 🎤 MYAAW VOICE 🎤 "))
	fmt.Println()

	// System instruction — dynamically include video awareness
	systemInstruction := "You are Myaaw, a friendly and helpful AI voice assistant. Keep your responses concise and conversational since you are speaking out loud. Be warm and natural."
	if len(videoModes) > 0 {
		systemInstruction += "\n\nYou have visual capabilities. "
		for _, mode := range videoModes {
			switch mode {
			case voice.VideoModeScreen:
				systemInstruction += "You can see the user's screen (a screenshot is sent to you periodically). "
			case voice.VideoModeCamera:
				systemInstruction += "You can see the user through their camera (a photo is sent to you periodically). "
			}
		}
		systemInstruction += "When asked about what you see, describe the visual content. You ARE able to see the screen/camera."
	}

	// Connect to Gemini Live API
	fmt.Println(theme.RenderSecondary("  Connecting to Gemini Live API..."))
	session, err := provider.NewGeminiLiveSession(config.LLMProviderAPIKey, systemInstruction)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  ❌ Failed to connect: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Make sure your Gemini API key is configured.\n\n")
		os.Exit(1)
	}
	defer session.Close()
	fmt.Println(theme.RenderSecondary("  ✓ Connected!"))
	fmt.Println()

	// Initialize PortAudio
	if err := portaudio.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to initialize PortAudio: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Make sure portaudio is installed: brew install portaudio\n\n")
		os.Exit(1)
	}
	defer portaudio.Terminate()

	// Setup microphone capture
	mic, err := voice.NewAudioCapture()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to open microphone: %v\n", err)
		os.Exit(1)
	}
	defer mic.Close()

	// Setup speaker output
	speaker, err := voice.NewAudioPlayer(0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to open speaker: %v\n", err)
		os.Exit(1)
	}
	defer speaker.Close()

	// Print status
	fmt.Print(theme.RenderSecondary("  🎤 Mic: "))
	fmt.Println("Ready")
	fmt.Print(theme.RenderSecondary("  🔊 Speaker: "))
	fmt.Println("Ready")

	if len(videoModes) > 0 {
		for _, mode := range videoModes {
			switch mode {
			case voice.VideoModeScreen:
				fmt.Print(theme.RenderSecondary("  🖥️  Screen: "))
				fmt.Printf("Capturing every %ds\n", videoInterval)
			case voice.VideoModeCamera:
				fmt.Print(theme.RenderSecondary("  📷 Camera: "))
				fmt.Printf("Capturing every %ds\n", videoInterval)
			}
		}
	}

	if transcript != nil {
		fmt.Print(theme.RenderSecondary("  📝 Transcript: "))
		fmt.Printf("voice-transcript-%s.log\n", time.Now().Format("2006-01-02"))
	}

	// Setup frame saving early so we can display info in status section
	var framesDir string
	if saveFrames && len(videoModes) > 0 {
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

	fmt.Println()
	fmt.Println(theme.RenderPrimary("  Start speaking! Press Ctrl+C to stop."))
	fmt.Println()

	// Show initial listening indicator
	fmt.Print("  🎤 Listening... ")

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	// State tracking for UI
	var uiMu sync.Mutex
	aiSpeaking := false
	lastUserText := ""
	lastAIText := ""

	// Response channel from Gemini
	responseChan := make(chan provider.LiveResponse, 100)

	// Start receiving responses from Gemini
	wg.Add(1)
	go func() {
		defer wg.Done()
		session.Receive(responseChan)
	}()

	// Start microphone capture goroutine
	mic.Start()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				data, err := mic.Read()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("Mic read error: %v", err)
					continue
				}
				// During AI speech, only send mic audio if it's loud enough
				// (user speaking directly into mic vs. speaker echo picked up)
				uiMu.Lock()
				speaking := aiSpeaking
				uiMu.Unlock()
				if speaking && !isLoudEnough(data, 1500) {
					continue // skip quiet audio (likely speaker echo)
				}
				if err := session.SendAudio(data); err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("Send audio error: %v", err)
				}
			}
		}
	}()

	// Start speaker playback + response handling goroutine
	speaker.Start()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-responseChan:
				if !ok {
					return
				}

				uiMu.Lock()
				switch resp.Type {
				case provider.LiveResponseUserSpeech:
					// Stream user speech inline
					if !aiSpeaking {
						if lastUserText == "" {
							fmt.Print("\r\033[K  🎤 You: ") // clear "Listening..." and print prefix
						}
						fmt.Print(resp.Text) // just append the delta
						lastUserText += resp.Text
					}

				case provider.LiveResponseModelSpeech:
					// Stream AI speech inline
					if lastUserText != "" && lastAIText == "" {
						// User finished, AI starting
						fmt.Println()
						fmt.Println() // blank line between user and AI
						if transcript != nil {
							transcript.LogUser(lastUserText)
						}
					}
					if lastAIText == "" {
						fmt.Print("\r\033[K  🐱 Myaaw: ") // clear indicator and print prefix
					}
					fmt.Print(resp.Text) // just append the delta
					lastAIText += resp.Text

				case provider.LiveResponseAudio:
					if !aiSpeaking {
						aiSpeaking = true
						if lastUserText != "" && lastAIText == "" {
							fmt.Println()
							fmt.Println()
							if transcript != nil {
								transcript.LogUser(lastUserText)
							}
						}
						if lastAIText == "" {
							fmt.Print("\r\033[K  ⏳ AI is responding...")
						}
					}
					if err := speaker.Play(resp.AudioData); err != nil {
						log.Printf("Speaker play error: %v", err)
					}

				case provider.LiveResponseText:
					if lastUserText != "" && lastAIText == "" {
						fmt.Println()
						if transcript != nil {
							transcript.LogUser(lastUserText)
						}
						lastUserText = ""
					}
					fmt.Printf("\n  🐱 %s", resp.Text)

				case provider.LiveResponseTurnDone:
					if lastAIText != "" {
						if transcript != nil {
							transcript.LogAI(lastAIText)
						}
					}
					lastAIText = ""
					lastUserText = ""
					aiSpeaking = false
					fmt.Println()
					fmt.Print("\n  🎤 Listening... ")
				}
				uiMu.Unlock()
			}
		}
	}()

	// Start video capture if enabled
	if len(videoModes) > 0 {

		capturer := voice.NewVideoCapturer(videoModes, time.Duration(videoInterval)*time.Second)
		frameChan := make(chan []byte, 5)

		wg.Add(1)
		go func() {
			defer wg.Done()
			capturer.Run(ctx, frameChan)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			frameCount := 0
			for {
				select {
				case <-ctx.Done():
					return
				case frame, ok := <-frameChan:
					if !ok {
						return
					}
					// Save frame to disk if enabled
					if framesDir != "" {
						frameCount++
						framePath := filepath.Join(framesDir, fmt.Sprintf("frame-%04d.jpg", frameCount))
						if err := os.WriteFile(framePath, frame, 0644); err != nil {
							log.Printf("Save frame error: %v", err)
						}
					}
					if err := session.SendVideoFrame(frame); err != nil {
						log.Printf("Send video frame error: %v", err)
					}
				}
			}
		}()
	}

	// Wait for shutdown signal
	<-sigChan
	fmt.Println("\n\n  👋 Stopping voice session...")
	cancel()

	// Give goroutines time to cleanup
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

	fmt.Println(theme.RenderSecondary("  Session ended. See you! 🐾"))
	fmt.Println()
}

func parseVideoModes(source string) []voice.VideoMode {
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

// isLoudEnough checks if the audio data (16-bit PCM, little-endian) exceeds
// the given RMS energy threshold. Used to filter out quiet speaker echo
// while still allowing loud direct speech (interruptions).
func isLoudEnough(data []byte, threshold int16) bool {
	numSamples := len(data) / 2
	if numSamples == 0 {
		return false
	}

	var sumSquares int64
	for i := 0; i < numSamples; i++ {
		sample := int16(data[i*2]) | int16(data[i*2+1])<<8
		sumSquares += int64(sample) * int64(sample)
	}

	rms := int16(math.Sqrt(float64(sumSquares) / float64(numSamples)))
	return rms > threshold
}
