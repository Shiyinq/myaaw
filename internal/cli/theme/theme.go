package theme

import (
	"github.com/charmbracelet/lipgloss"
)

// Color Palette based on MyAAW Mascot
const (
	ColorPrimary   = "#f7c087" // Peach/Orange
	ColorSecondary = "#a27264" // Brown
	ColorText      = "#f7f6e4" // Cream
	ColorMuted     = "#e9c8b9" // Light Brown/Pinkish
	ColorSuccess   = "#a6e3a1" // Green (Standard terminal green, adjusted if needed. Keeping standard for now or matching palette vibe?)
	// Let's stick to standard success/error but maybe tint them if we want full custom.
	// For now, using standard hexes that might complement the warm tones.
	ColorError = "#f38ba8" // Redish
	ColorDark  = "#1e1e2e" // Dark background for contrast if needed
)

var (
	// Base Text Style
	BaseStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorText))

	// Highlight / Primary Style
	HighlightStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorPrimary)).Bold(true)

	// Secondary Style
	SecondaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSecondary))

	// Muted / Dim Style
	MutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMuted)).Italic(true)

	// Success Style
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSuccess)).Bold(true)

	// Error Style
	ErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorError)).Bold(true)

	// Warning Style
	WarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")).Bold(true) // Hex for yellow/gold

	// Box Style (for panels)
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ColorPrimary)).
			Padding(0, 1)

	// Header Style
	HeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorDark)).
			Background(lipgloss.Color(ColorPrimary)).
			Bold(true).
			Padding(0, 1)
)

// Helper functions for quick rendering
func RenderPrimary(text string) string {
	return HighlightStyle.Render(text)
}

func RenderSecondary(text string) string {
	return SecondaryStyle.Render(text)
}

func RenderSuccess(text string) string {
	return SuccessStyle.Render(text)
}

func RenderError(text string) string {
	return ErrorStyle.Render(text)
}

func RenderWarning(text string) string {
	return WarningStyle.Render(text)
}

func RenderMuted(text string) string {
	return MutedStyle.Render(text)
}
