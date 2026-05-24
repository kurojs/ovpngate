package ui

import (
	"errors"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kurojs/ovpngate/internal/connect"
	"github.com/kurojs/ovpngate/internal/vpngate"
)

type phase int

const (
	phaseLoading phase = iota
	phaseList
	phaseDetail
	phaseConnecting
	phaseConnected
	phaseError
)

type msgServersFetched struct {
	servers []vpngate.Server
	err     error
}

type msgConnected struct {
	ip  string
	err error
}

type msgDisconnect struct{}

type msgCancelled struct{}

type msgTitleTick struct{}

type Model struct {
	phase      phase
	width      int
	height     int
	servers    []vpngate.Server
	filtered   []vpngate.Server
	cursor     int
	listScroll int
	filter     string

	filterCountry    string
	filterCountries  []string
	filterCountryIdx int

	titleAnim int

	connected  vpngate.Server
	assignedIP string
	spinner    spinner.Model
	err        error
	logs       []string
}

func InitialModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		phase:   phaseLoading,
		filter:  "all",
		spinner: s,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchServers(),
		titleTickCmd(),
	)
}

func titleTickCmd() tea.Cmd {
	return tea.Tick(1800*time.Millisecond, func(t time.Time) tea.Msg {
		return msgTitleTick{}
	})
}

func fetchServers() tea.Cmd {
	return func() tea.Msg {
		servers, err := vpngate.Fetch()
		return msgServersFetched{servers: servers, err: err}
	}
}

func connectToServer(s vpngate.Server) tea.Cmd {
	return func() tea.Msg {
		_, err := connect.Connect(s.HostName, s.OvpnConfig)
		if err != nil {
			return msgConnected{err: err}
		}

		ip, err := connect.WaitForTunnel()
		if errors.Is(err, connect.ErrCancelConnect) {
			return msgCancelled{}
		}
		return msgConnected{ip: ip, err: err}
	}
}

func disconnect() tea.Cmd {
	return func() tea.Msg {
		_ = connect.Disconnect()
		return msgDisconnect{}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case msgTitleTick:
		m.titleAnim = (m.titleAnim + 1) % 4
		return m, titleTickCmd()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			connect.Disconnect()
			return m, tea.Quit

		case "up", "k":
			if m.phase == phaseList && m.cursor > 0 {
				m.cursor--
				m.adjustListScroll()
			}

		case "down", "j":
			if m.phase == phaseList && m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.adjustListScroll()
			}

		case "pgup":
			if m.phase == phaseList {
				m.pageUp()
			}

		case "pgdown":
			if m.phase == phaseList {
				m.pageDown()
			}

		case "enter":
			if m.phase == phaseList {
				m.phase = phaseDetail
			} else if m.phase == phaseDetail && !m.isConnected() {
				m.phase = phaseConnecting
				m.logs = []string{"connecting..."}
				return m, connectToServer(m.filtered[m.cursor])
			}

		case "esc":
			switch m.phase {
			case phaseConnecting:
				connect.Cancel()
				m.phase = phaseDetail
				m.logs = append(m.logs, "cancelled")
			case phaseDetail:
				m.phase = phaseList
			}

		case "d":
			if m.phase == phaseConnected || m.phase == phaseDetail {
				return m, disconnect()
			}

		case "r":
			if m.phase == phaseList {
				m.phase = phaseLoading
				return m, tea.Batch(m.spinner.Tick, fetchServers())
			}

		case "a":
			if m.phase == phaseList {
				m.filter = "all"
				m.filterCountry = ""
				m.filterCountryIdx = 0
				m.applyFilter()
			}

		case "f":
			if m.phase == phaseList {
				m.filter = "fast"
				m.applyFilter()
			}

		case "c":
			if m.phase == phaseList {
				m.cycleCountry()
			}
		}

	case msgServersFetched:
		if msg.err != nil {
			m.phase = phaseError
			m.err = msg.err
			return m, nil
		}
		m.servers = msg.servers
		m.buildCountryList()
		m.applyFilter()
		m.phase = phaseList
		m.cursor = 0
		m.listScroll = 0

	case msgConnected:
		if m.phase != phaseConnecting {
			return m, nil
		}
		if msg.err != nil {
			m.phase = phaseDetail
			m.logs = append(m.logs, "error: "+msg.err.Error())
			return m, nil
		}
		m.connected = m.filtered[m.cursor]
		m.assignedIP = msg.ip
		m.phase = phaseConnected
		m.logs = append(m.logs, "connected with IP: "+msg.ip)

	case msgCancelled:
		if m.phase == phaseConnecting {
			m.phase = phaseDetail
		}
		return m, nil

	case msgDisconnect:
		m.connected = vpngate.Server{}
		m.assignedIP = ""
		m.phase = phaseList

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *Model) isConnected() bool {
	return m.connected.HostName != ""
}

func (m *Model) availableCountries() []string {
	seen := make(map[string]struct{})
	var countries []string
	for _, s := range m.servers {
		if _, ok := seen[s.CountryShort]; !ok && s.CountryShort != "" {
			seen[s.CountryShort] = struct{}{}
			countries = append(countries, s.CountryShort)
		}
	}
	sort.Strings(countries)
	return countries
}

func (m *Model) buildCountryList() {
	m.filterCountries = m.availableCountries()
	m.filterCountry = ""
	m.filterCountryIdx = 0
}

func (m *Model) cycleCountry() {
	if len(m.filterCountries) == 0 {
		return
	}
	m.filterCountryIdx = (m.filterCountryIdx + 1) % (len(m.filterCountries) + 1)
	if m.filterCountryIdx == 0 {
		m.filterCountry = ""
	} else {
		m.filterCountry = m.filterCountries[m.filterCountryIdx-1]
	}
	m.applyFilter()
}

func (m *Model) applyFilter() {
	m.filtered = make([]vpngate.Server, len(m.servers))
	copy(m.filtered, m.servers)

	if m.filterCountry != "" {
		var filtered []vpngate.Server
		for _, s := range m.filtered {
			if s.CountryShort == m.filterCountry {
				filtered = append(filtered, s)
			}
		}
		m.filtered = filtered
	}

	if m.filter == "fast" {
		sorted := make([]vpngate.Server, len(m.filtered))
		copy(sorted, m.filtered)
		for i := 0; i < len(sorted)-1; i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j].Speed > sorted[i].Speed {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		m.filtered = sorted
	}

	if len(m.filtered) == 0 {
		m.cursor = 0
		m.listScroll = 0
		return
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	m.adjustListScroll()
}

func (m Model) View() string {
	content := ""
	switch m.phase {
	case phaseLoading:
		content = "\n\n  " + m.spinner.View() + " fetching servers...\n"

	case phaseError:
		content = ErrorStyle.Render("\n  error: "+m.err.Error()) +
			"\n\n" + MutedStyle.Render("  q for exit")

	case phaseList:
		content = "\n" + renderList(m)

	case phaseDetail, phaseConnecting, phaseConnected:
		content = "\n" + renderDetail(m)
	}

	if m.width == 0 && m.height == 0 {
		return BaseStyle.Render(content)
	}
	w := m.width
	h := m.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}
	return BaseStyle.Render(lipgloss.Place(
		w, h,
		lipgloss.Center, lipgloss.Top,
		content,
	))
}
