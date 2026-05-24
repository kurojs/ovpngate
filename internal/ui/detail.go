package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderDetail(m Model) string {
	if len(m.filtered) == 0 {
		return ""
	}
	s := m.filtered[m.cursor]
	var b strings.Builder
	width := contentWidth(m)

	titleStyle := TitleStyle
	if m.titleAnim%2 == 1 {
		titleStyle = TitleStyleGreen
	}
	title := titleStyle.Render(countryFlag(s.CountryShort) + " " + s.CountryLong)
	b.WriteString(title + "\n")
	b.WriteString(SubtitleStyle.Render(s.HostName) + "\n")
	b.WriteString(MutedStyle.Render(s.IP) + "\n\n")

	stats := joinStats(
		statBlock("ping", pingStyle(s.Ping)),
		statBlock("speed", GreenStyle.Render(fmt.Sprintf("%dMbps", s.Speed))),
		statBlock("users", WarningStyle.Render(fmt.Sprintf("%d", s.Sessions))),
	)
	b.WriteString(stats + "\n\n")

	if s.Message != "" {
		b.WriteString(Divider(width-6) + "\n")
		b.WriteString(MutedStyle.Render("message") + "\n")
		b.WriteString(wrapText(s.Message, width-6) + "\n")
		b.WriteString("\n")
	}

	switch m.phase {
	case phaseConnecting:
		b.WriteString(WarningStyle.Render(m.spinner.View()+" connecting...") + "\n")
		b.WriteString(KeyHint("esc", "Cancel") + "\n\n")
	case phaseConnected:
		b.WriteString(GreenStyle.Render("\u25cf connected \u2014 "+m.assignedIP) + "\n")
		b.WriteString(KeyHint("d", "Disconnect") + "\n\n")
	default:
		b.WriteString(KeyHint("enter", "Connect") + "\n\n")
	}

	if len(m.logs) > 0 {
		b.WriteString(MutedStyle.Render("--- log ---") + "\n")
		logLines := m.logs
		if len(logLines) > 6 {
			logLines = logLines[len(logLines)-6:]
		}
		for _, l := range logLines {
			if strings.HasPrefix(strings.ToLower(l), "error:") {
				b.WriteString(ErrorStyle.Render(l) + "\n")
			} else {
				b.WriteString(MutedStyle.Render(l) + "\n")
			}
		}
		b.WriteString("\n")
	}

	hints := strings.Join([]string{
		KeyHint("esc", "Back"),
		KeyHint("q", "Quit"),
	}, "  ")
	b.WriteString(Divider(width-6) + "\n")
	b.WriteString(hints)

	return Panel(b.String(), width)
}

func statBlock(label, value string) string {
	return lipgloss.NewStyle().
		Width(14).
		Render(MutedStyle.Render(label) + "\n" + value)
}

func joinStats(cols ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func wrapText(text string, width int) string {
	if width < 10 {
		width = 10
	}
	return lipgloss.NewStyle().
		Width(width).
		Render(text)
}
