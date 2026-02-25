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
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	// Setup frame saving early
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
		}
	}

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	var aiSpeaking atomic.Bool

	// Initialize Bubble Tea Program
	p := tea.NewProgram(initialVoiceModel(videoModes, videoInterval, transcript, framesDir), tea.WithAltScreen())

	responseChan := make(chan provider.LiveResponse, 100)

	// Receiver
	wg.Add(1)
	go func() {
		defer wg.Done()
		session.Receive(responseChan)
	}()

	// Microphone — always sends audio to Gemini, server-side VAD handles interruption
	mic.Start()
	wg.Add(1)
	go func() {
		defer wg.Done()
		silenceFrames := 0
		isSpeaking := false

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

				// Skip mic input while AI is speaking (avoids sending speaker echo to Gemini)
				if aiSpeaking.Load() {
					continue
				}

				// Send audio to Gemini only when AI is NOT speaking
				if err := session.SendAudio(data); err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("Send audio error: %v", err)
				}

				loud := isLoudEnough(data, 800)
				if loud {
					silenceFrames = 0
					if !isSpeaking {
						isSpeaking = true
						p.Send(userLoudMsg{})
					}
				} else if isSpeaking {
					silenceFrames++
					if silenceFrames > 8 {
						isSpeaking = false
						p.Send(userQuietMsg{})
					}
				}
			}
		}
	}()

	// Speaker & AI logic
	speaker.Start()
	wg.Add(1)
	go func() {
		defer wg.Done()

		lastUserText := ""
		lastAISpoken := ""   // what the model actually says out loud
		lastAIThoughts := "" // internal reasoning/thinking text

		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-responseChan:
				if !ok {
					return
				}

				switch resp.Type {
				case provider.LiveResponseUserSpeech:
					lastUserText += resp.Text
					p.Send(userTextMsg(lastUserText))

				case provider.LiveResponseModelSpeech:
					if lastAISpoken == "" {
						p.Send(aiStartMsg{})
						aiSpeaking.Store(true)
					}
					lastAISpoken += resp.Text
					p.Send(aiTextMsg(lastAISpoken))

				case provider.LiveResponseAudio:
					if !aiSpeaking.Load() {
						p.Send(aiStartMsg{})
						aiSpeaking.Store(true)
					}
					if err := speaker.Play(resp.AudioData); err != nil {
						log.Printf("Speaker play error: %v", err)
					}

				case provider.LiveResponseText:
					lastAIThoughts += resp.Text

				case provider.LiveResponseTurnDone:
					if transcript != nil {
						if lastUserText != "" {
							transcript.LogUser(lastUserText)
						}
						if lastAISpoken != "" {
							transcript.LogAI(lastAISpoken)
						}
						if lastAIThoughts != "" {
							log.Printf("AI Thoughts: %s", lastAIThoughts)
						}
					}
					p.Send(turnDoneMsg{userText: lastUserText, aiText: lastAISpoken})
					lastUserText = ""
					lastAISpoken = ""
					lastAIThoughts = ""
					aiSpeaking.Store(false)
					p.Send(aiDoneMsg{})
				}
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

	// START TEA PROGRAM (blocks until ctrl+c)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running UI: %v\n", err)
	}

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

type uiState int

const (
	stateIdle uiState = iota
	stateUserSpeaking
	stateAIThinking
	stateAISpeaking
)

type tickMsg time.Time
type userLoudMsg struct{}
type userQuietMsg struct{}
type aiStartMsg struct{}
type aiDoneMsg struct{}
type userTextMsg string
type aiTextMsg string
type turnDoneMsg struct {
	userText string
	aiText   string
}

type voiceModel struct {
	state        uiState
	waveOffset   int
	width        int
	height       int
	quitting     bool
	currentUser  string // current turn user text (streaming)
	currentAI    string // current turn AI text (streaming)
	lastUserText string // last completed turn user text
	lastAIText   string // last completed turn AI text
	// Session info for bottom display
	videoModes    []voice.VideoMode
	videoInterval int
	transcriptLog string // transcript file path display
	framesDir     string // frames dir display
}

func initialVoiceModel(videoModes []voice.VideoMode, videoInterval int, transcript *voiceTranscriptLogger, framesDir string) voiceModel {
	transcriptLog := ""
	if transcript != nil {
		transcriptLog = "~/.myaaw/logs/voice-transcript-" + time.Now().Format("2006-01-02") + ".log"
	}
	framesDirDisplay := ""
	if framesDir != "" {
		homeDir, _ := os.UserHomeDir()
		framesDirDisplay = strings.TrimPrefix(framesDir, homeDir)
		framesDirDisplay = "~" + framesDirDisplay
	}
	return voiceModel{
		state:         stateIdle,
		videoModes:    videoModes,
		videoInterval: videoInterval,
		transcriptLog: transcriptLog,
		framesDir:     framesDirDisplay,
	}
}

func (m voiceModel) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m voiceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tickMsg:
		m.waveOffset++
		return m, tickCmd()
	case userLoudMsg:
		m.state = stateUserSpeaking
		m.lastUserText = "" // clear old user text when speaking again
	case userQuietMsg:
		if m.state == stateUserSpeaking {
			m.state = stateIdle
		}
	case aiStartMsg:
		m.state = stateAISpeaking
		m.currentUser = ""  // clear user text when AI starts
		m.lastUserText = "" // fully clear previous user text
	case aiDoneMsg:
		m.state = stateIdle
		m.currentAI = ""
		m.currentUser = ""
	case userTextMsg:
		m.currentUser = string(msg)
	case aiTextMsg:
		m.currentAI = string(msg)
	case turnDoneMsg:
		if msg.userText != "" {
			m.lastUserText = msg.userText
		}
		if msg.aiText != "" {
			m.lastAIText = msg.aiText
		}
	}
	return m, nil
}

func wordWrap(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	var result strings.Builder
	lineLen := 0
	words := strings.Fields(s)
	for i, word := range words {
		wLen := len(word)
		if i > 0 && lineLen+1+wLen > maxWidth {
			result.WriteString("\n")
			lineLen = 0
		}
		if lineLen > 0 {
			result.WriteString(" ")
			lineLen++
		}
		result.WriteString(word)
		lineLen += wLen
	}
	return result.String()
}

func (m voiceModel) View() string {
	if m.width == 0 || m.height == 0 || m.quitting {
		return ""
	}

	var text string
	var wave string

	waveChars := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	bars := make([]rune, 5)
	for i := 0; i < 5; i++ {
		idx := int((math.Sin(float64(m.waveOffset)/2.0+float64(i)) + 1.0) * 3.5)
		if idx < 0 {
			idx = 0
		}
		if idx > 7 {
			idx = 7
		}
		bars[i] = waveChars[idx]
	}
	wave = string(bars)

	switch m.state {
	case stateIdle:
		text = theme.RenderSecondary("Listening...")
	case stateUserSpeaking:
		text = theme.RenderSuccess(wave + " You are speaking...")
	case stateAIThinking:
		text = theme.RenderWarning("Myaaw is thinking...")
	case stateAISpeaking:
		text = theme.HighlightStyle.Render("🐱")
		text = lipgloss.JoinVertical(lipgloss.Center,
			text,
			theme.HighlightStyle.Render(wave+" Myaaw is speaking..."),
		)
	}

	statusLine := text

	// Build chat text section below the animation
	maxTextWidth := m.width / 2
	if maxTextWidth < 30 {
		maxTextWidth = 30
	}
	if maxTextWidth > 80 {
		maxTextWidth = 80
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	var chatLines []string

	// Show ONLY current speaker's text (not both at the same time)
	switch m.state {
	case stateUserSpeaking, stateIdle:
		// When user is speaking or idle, show user text
		if m.currentUser != "" {
			chatLines = append(chatLines, dimStyle.Render(wordWrap(m.currentUser, maxTextWidth)))
		} else if m.lastUserText != "" {
			chatLines = append(chatLines, dimStyle.Render(wordWrap(m.lastUserText, maxTextWidth)))
		}
	case stateAISpeaking, stateAIThinking:
		// When AI is speaking, show AI text
		if m.currentAI != "" {
			chatLines = append(chatLines, dimStyle.Render(wordWrap(m.currentAI, maxTextWidth)))
		} else if m.lastAIText != "" {
			chatLines = append(chatLines, dimStyle.Render(wordWrap(m.lastAIText, maxTextWidth)))
		}
	}

	chatSection := ""
	if len(chatLines) > 0 {
		// Left-align chat lines within a fixed-width centered box
		chatSection = lipgloss.JoinVertical(lipgloss.Left, chatLines...)
	}

	// Build info section (video modes, transcript, frames) — no emojis
	var infoLines []string
	for _, mode := range m.videoModes {
		switch mode {
		case voice.VideoModeScreen:
			infoLines = append(infoLines, dimStyle.Render(fmt.Sprintf("Screen: Capturing every %ds", m.videoInterval)))
		case voice.VideoModeCamera:
			infoLines = append(infoLines, dimStyle.Render(fmt.Sprintf("Camera: Capturing every %ds", m.videoInterval)))
		}
	}
	if m.transcriptLog != "" {
		infoLines = append(infoLines, dimStyle.Render("Transcript: "+m.transcriptLog))
	}
	if m.framesDir != "" {
		infoLines = append(infoLines, dimStyle.Render("Frames: "+m.framesDir))
	}

	infoSection := ""
	if len(infoLines) > 0 {
		infoSection = lipgloss.JoinVertical(lipgloss.Left, infoLines...)
	}

	instruction := dimStyle.Render("Press Ctrl+C to stop")

	// --- Layout: center animation + chat in the middle, info + instruction at bottom ---
	// Top section: animation + chat (vertically centered)
	var topParts []string
	topParts = append(topParts, statusLine)
	if chatSection != "" {
		topParts = append(topParts, "", lipgloss.PlaceHorizontal(lipgloss.Width(statusLine), lipgloss.Center, chatSection))
	}
	topBlock := lipgloss.JoinVertical(lipgloss.Center, topParts...)

	// Bottom section: info + ctrl+c (centered)
	var bottomParts []string
	if infoSection != "" {
		bottomParts = append(bottomParts, infoSection)
	}
	bottomParts = append(bottomParts, "", instruction)
	bottomBlock := lipgloss.JoinVertical(lipgloss.Center, bottomParts...)

	// Place top block in center of screen, bottom block at absolute bottom
	topBlockHeight := lipgloss.Height(topBlock)
	bottomBlockHeight := lipgloss.Height(bottomBlock)

	// Center the top block vertically (offset slightly above true center)
	topPad := (m.height - topBlockHeight - bottomBlockHeight) / 2
	if topPad < 1 {
		topPad = 1
	}

	// Gap between top and bottom
	gap := m.height - topPad - topBlockHeight - bottomBlockHeight
	if gap < 1 {
		gap = 1
	}

	// Center everything horizontally
	topCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, topBlock)
	bottomCentered := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bottomBlock)

	// Build full screen: top padding + top block + gap + bottom block
	var sb strings.Builder
	for i := 0; i < topPad; i++ {
		sb.WriteString("\n")
	}
	sb.WriteString(topCentered)
	for i := 0; i < gap; i++ {
		sb.WriteString("\n")
	}
	sb.WriteString(bottomCentered)

	return sb.String()
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
