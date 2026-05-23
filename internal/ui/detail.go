package ui

import (
	"fmt"
	"strings"
)

func renderDetail(m Model) string {
	if len(m.filtered) == 0 {
		return ""
	}

	s := m.filtered[m.cursor]
	var b strings.Builder

	b.WriteString(styleHeader.Render(countryFlag(s.CountryShort)+" "+s.CountryLong) + "\n")
	b.WriteString(styleNormal.Render(s.HostName) + "\n")
	b.WriteString(styleMuted.Render(s.IP) + "\n\n")

	b.WriteString(fmt.Sprintf("%s  %s  %s\n\n",
		styleMuted.Render("ping")+"\n"+pingStyle(s.Ping),
		styleMuted.Render("speed")+"\n"+styleGreen.Render(fmt.Sprintf("%dMbps", s.Speed)),
		styleMuted.Render("users")+"\n"+styleYellow.Render(fmt.Sprintf("%d", s.Sessions)),
	))

	if m.connected != nil && m.connected.HostName == s.HostName {
		b.WriteString(styleGreen.Render("● connected — "+m.assignedIP) + "\n")
		b.WriteString(styleNormal.Render("d → disconnect") + "\n\n")
	} else if m.phase == phaseConnecting {
		b.WriteString(styleYellow.Render(m.spinner.View()+" connecting...") + "\n\n")
	} else {
		b.WriteString(styleNormal.Render("enter → connect") + "\n\n")
	}

		if len(m.logs) > 0 {
		b.WriteString(styleMuted.Render("─── log ───") + "\n")
		for _, l := range m.logs {
			b.WriteString(styleMuted.Render(l) + "\n")
		}
	}

	b.WriteString("\n" + styleMuted.Render("esc → back"))

	return styleBorder.Render(b.String())
}
