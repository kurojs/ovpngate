package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kurojs/ovpngate/internal/vpngate"
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
		return GreenStyle.Render(s)
	}
	return WarningStyle.Render(s)
}

type colWidths struct {
	country int
	host    int
	ping    int
	speed   int
	users   int
}

func calcColWidths(servers []vpngate.Server) colWidths {
	w := colWidths{country: 7, host: 10, ping: 6, speed: 8, users: 5}
	for _, s := range servers {
		if cl := len(s.CountryLong); cl > w.country {
			w.country = cl
		}
		if hl := len(s.HostName); hl > w.host {
			w.host = hl
		}
		if pl := len(fmt.Sprintf("%dms", s.Ping)); pl > w.ping {
			w.ping = pl
		}
		if sl := len(fmt.Sprintf("%dMbps", s.Speed)); sl > w.speed {
			w.speed = sl
		}
		if ul := len(fmt.Sprintf("%d", s.Sessions)); ul > w.users {
			w.users = ul
		}
	}
	if w.country > 20 {
		w.country = 20
	}
	if w.host > 20 {
		w.host = 20
	}
	return w
}

func renderList(m Model) string {
	var b strings.Builder
	width := contentWidth(m)
	cw := calcColWidths(m.filtered)
	hasOffline := len(m.offlineFavorites) > 0

	titleStyle := TitleStyle
	if m.titleAnim%2 == 1 {
		titleStyle = TitleStyleGreen
	}
	headerParts := []string{
		titleStyle.Render("ovpngate"),
	}
	if m.filterCountry != "" {
		headerParts = append(headerParts, AccentStyle.Render(countryFlag(m.filterCountry)+" "+m.filterCountry))
	} else {
		headerParts = append(headerParts, MutedStyle.Render("all"))
	}
	if m.filter == "fast" {
		headerParts = append(headerParts, AccentStyle.Render("fast"))
	}
	if m.filter == "fav" && !hasOffline {
		headerParts = append(headerParts, AccentStyle.Render("fav"))
	}
	if m.filter == "fav" && hasOffline {
		headerParts = append(headerParts, GreenStyle.Render(fmt.Sprintf("fav (%d offline)", len(m.offlineFavorites))))
	}
	headerParts = append(headerParts,
		MutedStyle.Render(fmt.Sprintf("(%d)", len(m.filtered))),
	)
	b.WriteString("\n")
	b.WriteString(strings.Join(headerParts, " ") + "\n")
	b.WriteString(MutedStyle.Render("r ref  a all  f fast  v fav  c country  s star  enter  q quit") + "\n")
	b.WriteString(Divider(width-6) + "\n")

	if len(m.filtered) == 0 && !hasOffline {
		if m.filter == "fav" {
			b.WriteString(MutedStyle.Render("no favorites yet — press s to star a server"))
		} else {
			b.WriteString(MutedStyle.Render("no servers found"))
		}
		return Panel(b.String(), width)
	}

	visible := m.listVisibleCount()
	start := m.listScroll
	if start < 0 {
		start = 0
	}
	end := start + visible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	scrollTotal := len(m.filtered)
	scrollVisible := visible

	for i := start; i < end; i++ {
		s := m.filtered[i]
		flag := countryFlag(s.CountryShort)

		countryW := cw.country
		hostW := cw.host

		country := truncateText(s.CountryLong, countryW)
		host := truncateText(s.HostName, hostW)

		isOffline := m.offlineSet[s.IP]

		var ping, speed, users string
		if isOffline {
			ping = MutedStyle.Render("-")
			speed = MutedStyle.Render("-")
			users = MutedStyle.Render("-")
		} else {
			ping = fmt.Sprintf("%dms", s.Ping)
			speed = fmt.Sprintf("%dMbps", s.Speed)
			users = fmt.Sprintf("%d", s.Sessions)
		}

		operator := s.Operator
		if operator == "" {
			operator = "-"
		}

		base := fmt.Sprintf("%s %-*s %-*s %*s %*s %*s",
			flag, countryW, country, hostW, host,
			cw.ping, ping, cw.speed, speed, cw.users, users,
		)

		innerWidth := width - 6
		opWidth := innerWidth - lipgloss.Width(base) - 2
		if opWidth < 4 {
			opWidth = 4
		}
		line := base + "  " + truncateText(operator, opWidth)

		isFav := m.favStore.IsFavorite(s.IP)
		selected := i == m.cursor

		if isOffline {
			item := OfflineItem(line)
			scrollMark := ""
			if scrollTotal > scrollVisible {
				pos := float64(m.cursor) / float64(scrollTotal-1)
				barIdx := int(pos * float64(scrollVisible-1))
				if i-start == barIdx {
					scrollMark = " " + AccentStyle.Render("█")
				}
			}
			b.WriteString(item + scrollMark + "\n")
			continue
		}

		item := ListItem(line, selected, isFav)

		scrollMark := ""
		if scrollTotal > scrollVisible {
			pos := float64(m.cursor) / float64(scrollTotal-1)
			barIdx := int(pos * float64(scrollVisible-1))
			if i-start == barIdx {
				scrollMark = " " + AccentStyle.Render("█")
			}
		}
		b.WriteString(item + scrollMark + "\n")
	}

	pos := fmt.Sprintf("%d-%d of %d", start+1, end, len(m.filtered))
	b.WriteString(Divider(width-6) + "\n")
	b.WriteString(MutedStyle.Render(pos))

	return Panel(b.String(), width)
}

func (m Model) listVisibleCount() int {
	if m.height <= 0 {
		return 10
	}
	visible := m.height - 12
	if visible < 6 {
		visible = 6
	}
	return visible
}

func (m *Model) adjustListScroll() {
	visible := m.listVisibleCount()
	if m.cursor < m.listScroll {
		m.listScroll = m.cursor
	}
	if m.cursor >= m.listScroll+visible {
		m.listScroll = m.cursor - visible + 1
	}
	if m.listScroll < 0 {
		m.listScroll = 0
	}
	maxScroll := len(m.filtered) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.listScroll > maxScroll {
		m.listScroll = maxScroll
	}
}

func (m *Model) pageUp() {
	visible := m.listVisibleCount()
	if m.cursor > 0 {
		m.cursor -= visible
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.adjustListScroll()
	}
}

func (m *Model) pageDown() {
	visible := m.listVisibleCount()
	if m.cursor < len(m.filtered)-1 {
		m.cursor += visible
		if m.cursor > len(m.filtered)-1 {
			m.cursor = len(m.filtered) - 1
		}
		m.adjustListScroll()
	}
}

func contentWidth(m Model) int {
	if m.width == 0 {
		return 72
	}
	w := m.width - 4
	if w < 48 {
		w = 48
	}
	if w > 96 {
		w = 96
	}
	return w
}

func truncateText(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
