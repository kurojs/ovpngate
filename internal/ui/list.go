package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	styleCursor  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	styleNormal  = lipgloss.NewStyle().Foreground(lipgloss.Color("#c0caf5"))
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	styleGreen   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	styleYellow  = lipgloss.NewStyle().Foreground(lipgloss.Color("#e0af68"))
	styleHeader  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7")).Bold(true)
	styleBorder  = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#414868")).
			Padding(0, 1)
)

func countryFlag(cc string) string {
	flags := map[string]string{
		"JP": "🇯🇵", "US": "🇺🇸", "KR": "🇰🇷",
		"NL": "🇳🇱", "DE": "🇩🇪", "FR": "🇫🇷",
		"GB": "🇬🇧", "CA": "🇨🇦", "AU": "🇦🇺",
		"TW": "🇹🇼", "SG": "🇸🇬", "IN": "🇮🇳",
	}
	if f, ok := flags[cc]; ok {
		return f
	}
	return "🌐"
}

func pingStyle(ping int) string {
	s := fmt.Sprintf("%dms", ping)
	if ping < 30 {
		return styleGreen.Render(s)
	}
	return styleYellow.Render(s)
}

func renderList(m Model) string {
	var b strings.Builder
	
	b.WriteString(styleHeader.Render("ovpngate") + "\n")
	b.WriteString(styleMuted.Render("r refresh · a all · f fast · enter select · q quit") + "\n\n")

	if len(m.filtered) == 0 {
		b.WriteString(styleMuted.Render("no servers found"))
		return b.String()
	}

	for i, s := range m.filtered {
		flag := countryFlag(s.CountryShort)
		line := fmt.Sprintf("%s %-30s %s  %dMbps  %d users",
			flag,
			s.HostName,
			pingStyle(s.Ping),
			s.Speed,
			s.Sessions,
		)

		if i == m.cursor {
			b.WriteString(styleCursor.Render("▶ "+line) + "\n")
		} else {
			b.WriteString(styleNormal.Render("  "+line) + "\n")
		}
	}

	return styleBorder.Render(b.String())
}
