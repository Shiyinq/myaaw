package service

import (
	"math"
	"strings"
)

// Settings holds the dashboard preferences persisted in ~/.myaaw/controlplane/controlplane.db
// (settings table) so they stay in sync across browsers.
// Defaults match the WebUI's built-in defaults.
type Settings struct {
	Locale              string  `json:"locale"`
	ThemeMode           string  `json:"theme_mode"`
	Theme               string  `json:"theme"`
	Density             string  `json:"density"`
	TextScale           int     `json:"text_scale"`
	ReduceMotion        bool    `json:"reduce_motion"`
	ShellMode           string  `json:"shell_mode"`
	FocusAgent          string  `json:"focus_agent"`
	SendKey             string  `json:"send_key"`
	AutoScroll          bool    `json:"auto_scroll"`
	Steps               string  `json:"steps"`
	KeepDrafts          bool    `json:"keep_drafts"`
	QueueMessages       bool    `json:"queue_messages"`
	InterruptOnSend     bool    `json:"interrupt_on_send"`
	SpeakResponses      bool    `json:"speak_responses"`
	SpeechRate          float64 `json:"speech_rate"`
	SpeechLang          string  `json:"speech_lang"`
	SpeechVoice         string  `json:"speech_voice"`
	DictationLang       string  `json:"dictation_lang"`
	NotifyEnabled       bool    `json:"notify_enabled"`
	NotifyBrowser       bool    `json:"notify_browser"`
	NotifySound         string  `json:"notify_sound"`
	NotifyVolume        float64 `json:"notify_volume"`
	ShowConnectionBadge bool    `json:"show_connection_badge"`
	ShowHostBadge       bool    `json:"show_host_badge"`
	ShowRunMascot       bool    `json:"show_run_mascot"`
}

// SettingsPatch carries optional fields for partial updates (PATCH /settings)
// and tolerant loading of older stored settings JSON payloads.
type SettingsPatch struct {
	Locale              *string  `json:"locale,omitempty"`
	ThemeMode           *string  `json:"theme_mode,omitempty"`
	Theme               *string  `json:"theme,omitempty"`
	Density             *string  `json:"density,omitempty"`
	TextScale           *int     `json:"text_scale,omitempty"`
	ReduceMotion        *bool    `json:"reduce_motion,omitempty"`
	ShellMode           *string  `json:"shell_mode,omitempty"`
	FocusAgent          *string  `json:"focus_agent,omitempty"`
	SendKey             *string  `json:"send_key,omitempty"`
	AutoScroll          *bool    `json:"auto_scroll,omitempty"`
	Steps               *string  `json:"steps,omitempty"`
	KeepDrafts          *bool    `json:"keep_drafts,omitempty"`
	QueueMessages       *bool    `json:"queue_messages,omitempty"`
	InterruptOnSend     *bool    `json:"interrupt_on_send,omitempty"`
	SpeakResponses      *bool    `json:"speak_responses,omitempty"`
	SpeechRate          *float64 `json:"speech_rate,omitempty"`
	SpeechLang          *string  `json:"speech_lang,omitempty"`
	SpeechVoice         *string  `json:"speech_voice,omitempty"`
	DictationLang       *string  `json:"dictation_lang,omitempty"`
	NotifyEnabled       *bool    `json:"notify_enabled,omitempty"`
	NotifyBrowser       *bool    `json:"notify_browser,omitempty"`
	NotifySound         *string  `json:"notify_sound,omitempty"`
	NotifyVolume        *float64 `json:"notify_volume,omitempty"`
	ShowConnectionBadge *bool    `json:"show_connection_badge,omitempty"`
	ShowHostBadge       *bool    `json:"show_host_badge,omitempty"`
	ShowRunMascot       *bool    `json:"show_run_mascot,omitempty"`
}

// DefaultSettings returns the canonical default settings matching the WebUI.
func DefaultSettings() Settings {
	return Settings{
		Locale:          "en",
		ThemeMode:       "system",
		Theme:           "",
		Density:         "comfortable",
		TextScale:       100,
		ReduceMotion:    false,
		ShellMode:       "studio",
		FocusAgent:      "",
		SendKey:         "enter",
		AutoScroll:      true,
		Steps:           "summary",
		KeepDrafts:      true,
		QueueMessages:   false,
		InterruptOnSend: true,
		SpeakResponses:  false,
		SpeechRate:      1,
		SpeechLang:      "auto",
		SpeechVoice:     "",
		DictationLang:   "app",
		NotifyEnabled:   true,
		NotifyBrowser:   false,
		NotifySound:     "chime",
		NotifyVolume:    0.7,
		ShowHostBadge:   true,
		ShowRunMascot:   true,
	}
}

// Apply merges non-nil fields from patch onto s and normalizes the result.
func (s Settings) Apply(patch SettingsPatch) Settings {
	next := s
	if patch.Locale != nil {
		next.Locale = *patch.Locale
	}
	if patch.ThemeMode != nil {
		next.ThemeMode = *patch.ThemeMode
	}
	if patch.Theme != nil {
		next.Theme = *patch.Theme
	}
	if patch.Density != nil {
		next.Density = *patch.Density
	}
	if patch.TextScale != nil {
		next.TextScale = *patch.TextScale
	}
	if patch.ReduceMotion != nil {
		next.ReduceMotion = *patch.ReduceMotion
	}
	if patch.ShellMode != nil {
		next.ShellMode = *patch.ShellMode
	}
	if patch.FocusAgent != nil {
		next.FocusAgent = *patch.FocusAgent
	}
	if patch.SendKey != nil {
		next.SendKey = *patch.SendKey
	}
	if patch.AutoScroll != nil {
		next.AutoScroll = *patch.AutoScroll
	}
	if patch.Steps != nil {
		next.Steps = *patch.Steps
	}
	if patch.KeepDrafts != nil {
		next.KeepDrafts = *patch.KeepDrafts
	}
	if patch.QueueMessages != nil {
		next.QueueMessages = *patch.QueueMessages
		if *patch.QueueMessages && patch.InterruptOnSend == nil {
			next.InterruptOnSend = false
		}
	}
	if patch.InterruptOnSend != nil {
		next.InterruptOnSend = *patch.InterruptOnSend
		if *patch.InterruptOnSend && patch.QueueMessages == nil {
			next.QueueMessages = false
		}
	}
	if patch.SpeakResponses != nil {
		next.SpeakResponses = *patch.SpeakResponses
	}
	if patch.SpeechRate != nil {
		next.SpeechRate = *patch.SpeechRate
	}
	if patch.SpeechLang != nil {
		next.SpeechLang = *patch.SpeechLang
	}
	if patch.SpeechVoice != nil {
		next.SpeechVoice = *patch.SpeechVoice
	}
	if patch.DictationLang != nil {
		next.DictationLang = *patch.DictationLang
	}
	if patch.NotifyEnabled != nil {
		next.NotifyEnabled = *patch.NotifyEnabled
	}
	if patch.NotifyBrowser != nil {
		next.NotifyBrowser = *patch.NotifyBrowser
	}
	if patch.NotifySound != nil {
		next.NotifySound = *patch.NotifySound
	}
	if patch.NotifyVolume != nil {
		next.NotifyVolume = *patch.NotifyVolume
	}
	if patch.ShowConnectionBadge != nil {
		next.ShowConnectionBadge = *patch.ShowConnectionBadge
	}
	if patch.ShowHostBadge != nil {
		next.ShowHostBadge = *patch.ShowHostBadge
	}
	if patch.ShowRunMascot != nil {
		next.ShowRunMascot = *patch.ShowRunMascot
	}
	return next.Normalize()
}

// Normalize clamps numeric ranges and replaces unknown enum values with defaults.
func (s Settings) Normalize() Settings {
	def := DefaultSettings()

	switch s.Locale {
	case "id", "en":
	default:
		s.Locale = def.Locale
	}

	switch s.ThemeMode {
	case "light", "dark", "system":
	default:
		s.ThemeMode = def.ThemeMode
	}

	s.Theme = strings.TrimSpace(s.Theme)
	if len(s.Theme) > 64 {
		s.Theme = ""
	}

	switch s.Density {
	case "comfortable", "compact":
	default:
		s.Density = def.Density
	}

	if s.TextScale < 100 {
		s.TextScale = 100
	} else if s.TextScale > 125 {
		s.TextScale = 125
	}

	switch s.ShellMode {
	case "studio", "classic", "focus", "bots":
	case "advanced", "standard":
		s.ShellMode = "studio"
	case "simple":
		s.ShellMode = "classic"
	default:
		s.ShellMode = def.ShellMode
	}

	s.FocusAgent = strings.TrimSpace(s.FocusAgent)

	switch s.SendKey {
	case "enter", "mod-enter":
	default:
		s.SendKey = def.SendKey
	}

	switch s.Steps {
	case "summary", "expanded":
	default:
		s.Steps = def.Steps
	}

	// Queue and interrupt are mutually exclusive; queue wins when both are set.
	if s.QueueMessages && s.InterruptOnSend {
		s.InterruptOnSend = false
	}

	if math.IsNaN(s.SpeechRate) || math.IsInf(s.SpeechRate, 0) || s.SpeechRate == 0 {
		s.SpeechRate = def.SpeechRate
	}
	s.SpeechRate = math.Round(s.SpeechRate*100) / 100
	if s.SpeechRate < 0.75 {
		s.SpeechRate = 0.75
	} else if s.SpeechRate > 1.5 {
		s.SpeechRate = 1.5
	}

	switch s.SpeechLang {
	case "auto", "app", "id", "en":
	default:
		s.SpeechLang = def.SpeechLang
	}

	switch s.DictationLang {
	case "app", "id", "en":
	default:
		s.DictationLang = def.DictationLang
	}

	s.NotifySound = strings.TrimSpace(s.NotifySound)
	if s.NotifySound == "" {
		s.NotifySound = def.NotifySound
	}

	if math.IsNaN(s.NotifyVolume) || math.IsInf(s.NotifyVolume, 0) {
		s.NotifyVolume = def.NotifyVolume
	}
	s.NotifyVolume = math.Round(s.NotifyVolume*100) / 100
	if s.NotifyVolume < 0 {
		s.NotifyVolume = 0
	} else if s.NotifyVolume > 1 {
		s.NotifyVolume = 1
	}

	return s
}

// Settings returns the current persisted dashboard settings.
func (s *AuthService) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds.Settings
}

// UpdateSettings merges patch into the stored settings and writes to controlplane.db.
func (s *AuthService) UpdateSettings(patch SettingsPatch) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.creds.Settings = s.creds.Settings.Apply(patch)
	if err := s.saveSettingsLocked(); err != nil {
		return Settings{}, err
	}
	return s.creds.Settings, nil
}
