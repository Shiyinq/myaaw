package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"myaaw/internal/agent"
	"myaaw/internal/cli/theme"
	"myaaw/internal/config"
	"myaaw/internal/provider"
	"myaaw/internal/voice"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	// Setup logging
	logDir := filepath.Join(homeDir, ".myaaw", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatal("Error creating log directory:", err)
	}
	logPath := filepath.Join(logDir, "myaaw-voice-classic.log")
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Error opening log file: %v", err)
	}
	defer f.Close()
	log.SetOutput(f)

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

	if config.TranscriberProviderName == "" || config.TranscriberAPIKey == "" {
		fmt.Fprintf(os.Stderr, "  ❌ Error: Transcriber is not configured.\n")
		fmt.Fprintf(os.Stderr, "  Please configure the 'transcriber' block in your config.json.\n")
		os.Exit(1)
	}

	if config.SynthesizerProviderName == "" || config.SynthesizerAPIKey == "" {
		fmt.Fprintf(os.Stderr, "  ❌ Error: Synthesizer is not configured.\n")
		fmt.Fprintf(os.Stderr, "  Please configure the 'synthesizer' block in your config.json.\n")
		os.Exit(1)
	}

	// Create Synthesizer (TTS) via factory
	synthesizer, err := provider.CreateSynthesizer(config.SynthesizerProviderName, config.SynthesizerAPIKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ❌ Failed to create TTS synthesizer: %v\n", err)
		os.Exit(1)
	}

	// Parse video modes
	videoModes := parseVCVideoModes(vcVideoSource)

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
		}
	}

	// Create VAD (Voice Activity Detection)
	vad := voice.NewVAD(800, 1500*time.Millisecond)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	// Initialize Bubble Tea Program
	p := tea.NewProgram(initialVoiceClassicModel(videoModes, vcVideoInterval, transcript, framesDir), tea.WithAltScreen())

	// Main conversation loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		var audioBuffer []byte

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
				p.Send(userLoudMsg{})
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
					p.Send(userQuietMsg{})
					continue
				}

				p.Send(userQuietMsg{})
				p.Send(aiThinkingMsg{})

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
					p.Send(aiDoneMsg{})
					audioBuffer = nil
					continue
				}

				if strings.TrimSpace(transcribedText) == "" {
					p.Send(aiDoneMsg{})
					audioBuffer = nil
					continue
				}

				if transcript != nil {
					transcript.LogUser(transcribedText)
				}
				p.Send(userTextMsg(transcribedText))

				// 2. Agent Loop: Send to Agent directly
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

				var responseText string
				var responseThought string
				var responseTrace []provider.ReactStep

				err = ag.RunStream(modelName, conversationHistory, func(partial provider.Message) error {
					if partial.Thought != "" {
						responseThought += partial.Thought
						p.Send(aiStreamThoughtMsg{thought: responseThought, trace: partial.Trace})
					}
					if len(partial.Trace) > 0 {
						responseTrace = partial.Trace
						// Also update even if thought didn't change, for showing tools
						p.Send(aiStreamThoughtMsg{thought: responseThought, trace: partial.Trace})
					}

					if partial.Content != nil {
						if str, ok := partial.Content.(string); ok {
							responseText += str
						}
					}
					return nil
				})

				if err != nil {
					log.Printf("Agent error: %v", err)
					p.Send(aiDoneMsg{})
					audioBuffer = nil
					continue
				}

				response := provider.Message{
					Role:    "assistant",
					Content: responseText,
					Thought: responseThought,
					Trace:   responseTrace,
				}

				// Add assistant response to history
				conversationHistory = append(conversationHistory, response)
				if transcript != nil {
					transcript.LogAI(responseText)
				}
				p.Send(aiTextMsg(responseText))

				// 3. TTS: Convert response to audio
				p.Send(aiGeneratingAudioMsg{})

				audioData, _, err := synthesizer.Synthesize(ctx, responseText)
				if err != nil {
					log.Printf("TTS error: %v", err)
					p.Send(aiDoneMsg{})
				} else {
					// Save AI audio if enabled
					if audioDir != "" {
						aiAudioPath := filepath.Join(audioDir, fmt.Sprintf("turn-%03d-ai.wav", turnCounter))
						saveWAV(aiAudioPath, audioData, 24000)
					}

					// Play audio
					p.Send(aiStartMsg{})
					speaker.Play(audioData)

					// Wait for playback to finish
					for speaker.IsPlaying() {
						time.Sleep(100 * time.Millisecond)
					}
				}

				p.Send(turnDoneMsg{userText: transcribedText, aiText: responseText})
				p.Send(aiDoneMsg{})

				// Restart mic and reset VAD
				mic.Start()
				vad.Reset()

				audioBuffer = nil

			case voice.VADSilence:
				// Do nothing, keep listening
			}
		}
	}()

	// START TEA PROGRAM (blocks until ctrl+c)
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running UI: %v\n", err)
	}

	fmt.Println("\n\n  👋 Stopping voice session...")
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

	fmt.Println(theme.RenderSecondary("  Session ended. See you! 🐾"))
	fmt.Println()
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

type aiThinkingMsg struct{}

type aiGeneratingAudioMsg struct{}

type aiStreamThoughtMsg struct {
	thought string
	trace   []provider.ReactStep
}

type voiceClassicModel struct {
	viewport         viewport.Model
	state            uiState
	waveOffset       int
	width            int
	height           int
	quitting         bool
	currentUser      string // current turn user text (streaming)
	currentAI        string // current turn AI text (streaming)
	lastUserText     string // last completed turn user text
	lastAIText       string // last completed turn AI text
	streamingThought string // current thought/tool trace
	videoModes       []voice.VideoMode
	videoInterval    int
	transcriptLog    string // transcript file path display
	framesDir        string // frames dir display
}

func initialVoiceClassicModel(videoModes []voice.VideoMode, videoInterval int, transcript *voiceTranscriptLogger, framesDir string) voiceClassicModel {
	vp := viewport.New(80, 20)
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
	return voiceClassicModel{
		viewport:      vp,
		state:         stateIdle,
		videoModes:    videoModes,
		videoInterval: videoInterval,
		transcriptLog: transcriptLog,
		framesDir:     framesDirDisplay,
	}
}

func (m voiceClassicModel) Init() tea.Cmd {
	return vcTickCmd()
}

func vcTickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m voiceClassicModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = m.height - 15
	case tickMsg:
		m.waveOffset++
		cmds = append(cmds, vcTickCmd())
	case userLoudMsg:
		m.state = stateUserSpeaking
		m.lastUserText = ""
	case userQuietMsg:
		if m.state == stateUserSpeaking {
			m.state = stateIdle
		}
	case aiThinkingMsg:
		m.state = stateAIThinking
		m.streamingThought = "" // Clear previous thoughts
	case aiStreamThoughtMsg:
		text := strings.TrimSpace(msg.thought)
		for _, step := range msg.trace {
			// Show the currently executing tool action based on the trace
			text += fmt.Sprintf("\nUsing %s...", step.Action)
			// If it has observation, we could show completed, but standard is just showing it's used
		}
		m.streamingThought = text
	case aiGeneratingAudioMsg:
		m.state = stateAIGeneratingAudio
		m.streamingThought = "" // clear thoughts
	case aiStartMsg:
		m.state = stateAISpeaking
		m.currentUser = ""      // clear user text when AI starts
		m.lastUserText = ""     // fully clear previous user text
		m.streamingThought = "" // safely clear thoughts as well

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

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	if vpCmd != nil {
		cmds = append(cmds, vpCmd)
	}

	m.viewport.SetContent(m.buildChatContent())

	switch msg.(type) {
	case userTextMsg, aiTextMsg, aiStreamThoughtMsg:
		m.viewport.GotoBottom()
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m voiceClassicModel) buildChatContent() string {
	maxTextWidth := m.width / 2
	if maxTextWidth < 30 {
		maxTextWidth = 30
	}
	if maxTextWidth > 80 {
		maxTextWidth = 80
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	var chatLines []string

	switch m.state {
	case stateUserSpeaking, stateIdle:
		if m.currentUser != "" {
			chatLines = append(chatLines, dimStyle.Render(wordWrap(m.currentUser, maxTextWidth)))
		} else if m.lastUserText != "" {
			chatLines = append(chatLines, dimStyle.Render(wordWrap(m.lastUserText, maxTextWidth)))
		}
	case stateAIThinking:
		if m.streamingThought != "" {
			chatLines = append(chatLines, dimStyle.Render(wordWrap(m.streamingThought, maxTextWidth)))
		} else {
			chatLines = append(chatLines, dimStyle.Render("Thinking..."))
		}
	case stateAIGeneratingAudio:
		// Keep showing the thought process while generating audio
		if m.streamingThought != "" {
			chatLines = append(chatLines, dimStyle.Render(wordWrap(m.streamingThought, maxTextWidth)))
		}
		if m.currentAI != "" {
			chatLines = append(chatLines, lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(wordWrap(m.currentAI, maxTextWidth)))
		} else {
			chatLines = append(chatLines, dimStyle.Render("Processing response..."))
		}
	case stateAISpeaking:
		if m.currentAI != "" {
			chatLines = append(chatLines, lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(wordWrap(m.currentAI, maxTextWidth)))
		} else if m.lastAIText != "" {
			chatLines = append(chatLines, lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Render(wordWrap(m.lastAIText, maxTextWidth)))
		}
	}

	if len(chatLines) > 0 {
		// Join the lines, then split them to center each constructed line individually
		joined := lipgloss.JoinVertical(lipgloss.Top, chatLines...)
		lines := strings.Split(joined, "\n")
		var centeredLines []string
		for _, line := range lines {
			centeredLines = append(centeredLines, lipgloss.PlaceHorizontal(m.width, lipgloss.Center, line))
		}
		return strings.Join(centeredLines, "\n")
	}
	return ""
}

func (m voiceClassicModel) View() string {
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
	case stateAIGeneratingAudio:
		text = theme.RenderSecondary("Generating speech...")
	case stateAISpeaking:
		text = theme.HighlightStyle.Render("🐱")
		text = lipgloss.JoinVertical(lipgloss.Center,
			text,
			theme.HighlightStyle.Render(wave+" Myaaw is speaking..."),
		)
	}

	statusLine := text

	// Build chat text section below the animation
	chatSection := m.viewport.View()

	// Build info section (video modes, transcript, frames) — no emojis
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
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
	topParts = append(topParts, lipgloss.PlaceHorizontal(m.width, lipgloss.Center, statusLine))
	if chatSection != "" {
		topParts = append(topParts, "", lipgloss.PlaceHorizontal(m.width, lipgloss.Center, chatSection))
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
