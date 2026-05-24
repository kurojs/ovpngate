package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	colorText        = "#F3F6F9"
	colorTextMuted   = "#5C6170"
	colorPurpleLight = "#C4B5FD"
	colorPurple      = "#A78BFA"
	colorPurpleDim   = "#8B7CC0"
	colorGreen       = "#86EFAC"
	colorWarning     = "#FCD34D"
	colorError       = "#FCA5A5"
	colorBorder      = "#1E293B"
)

var (
	BaseStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText))

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPurpleLight))

	TitleStyleAlt = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPurple))

	TitleStyleGreen = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorGreen))

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextMuted))

	AccentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPurple))

	GreenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGreen))

	MutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorTextMuted))

	WarningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorWarning))

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError))
)

func Panel(content string, width int) string {
	style := lipgloss.NewStyle().Padding(0, 2)
	if width > 0 {
		style = style.Width(width)
	}
	return style.Render(content)
}

func Divider(width int) string {
	if width <= 0 {
		width = 34
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBorder)).
		Render(strings.Repeat("─", width))
}

func KeyHint(key, desc string) string {
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorPurple)).
		Bold(true).
		Render(key)
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorTextMuted)).
		Render(desc)
	return keyStyle + " " + descStyle
}

func ListItem(label string, selected, isFav bool) string {
	prefix := "  "
	if isFav {
		prefix = "★ "
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorText))
	if selected {
		style = style.Foreground(lipgloss.Color(colorPurpleLight))
	}
	return style.Render(prefix + label)
}

func OfflineItem(label string) string {
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorTextMuted)).
		Render("  " + label + " (offline)")
}
