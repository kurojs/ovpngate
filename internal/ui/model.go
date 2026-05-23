package ui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
	err error
}

type msgConnected struct {
	ip string
	err error
}

type msgDisconnect struct {
	err error
}

type Model struct {
	phase phase
	servers []vpngate.Server
	filtered []vpngate.Server
	cursor int
	filter string
	connected *vpngate.Server
	assignedIP string
	spinner spinner.Model
	err error
	logs []string
}

func InitialModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		phase: phaseLoading,
		filter: "all",
		spinner: s,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchServers(),
	)
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
		return msgConnected{ip: ip, err: err}
	}
}

func disconnect() tea.Cmd {
	return func() tea.Msg {
		err := connect.Disconnect()
		return msgDisconnect{err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

		case tea.KeyMsg:
			switch msg.String() {
				case "ctrl+c", "q":
					if m.connected != nil {
						connect.Disconnect()
					}
					return m, tea.Quit

		case "up", "k":
			if m.phase == phaseList && m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.phase == phaseList && m.cursor < len(m.filtered)-1 {
				m.cursor++
			}

		case "enter":
			if m.phase == phaseList  {
				m.phase = phaseDetail
			} else if m.phase == phaseDetail && m.connected == nil {
				m.phase = phaseConnecting
				m.logs = []string{"connecting..."}
				return m, connectToServer(m.filtered[m.cursor])
			}

		case "esc":
			if m.phase == phaseDetail {
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
			m.filter = "all"
			m.applyFilter()
		case "f":
			m.filter = "fast"
			m.applyFilter()
		}

		case msgServersFetched:
			if msg.err != nil {
				m.phase = phaseError
				m.err = msg.err
				return m, nil
			}
			m.servers = msg.servers
			m.applyFilter()
			m.phase = phaseList
			m.cursor = 0

		case msgConnected:
			if msg.err != nil {
				m.phase = phaseDetail
				m.logs = append(m.logs, "error: "+msg.err.Error())
				return m, nil
			}
			m.connected = &m.filtered[m.cursor]
			m.assignedIP = msg.ip
			m.phase = phaseConnected
			m.logs = append(m.logs, "connected with IP: "+msg.ip)
			
		case msgDisconnect:
			m.connected = nil
			m.assignedIP = ""
			m.phase = phaseList

		case spinner.TickMsg:
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
	}
	
	return m, nil

}
	
func (m *Model) applyFilter() {
	switch m.filter {
	case "all":
		m.filtered = m.servers
	case "fast":
		sorted := make([]vpngate.Server, len(m.servers))
		copy(sorted, m.servers)
		for i := 0; i < len(sorted)-1; i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j].Speed > sorted[i].Speed {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		m.filtered = sorted
	}
}

func (m Model) View() string {
	switch m.phase {
	case phaseLoading:
		return "\n\n  " + m.spinner.View() + " fetching servers...\n"

	case phaseError:
		return styleYellow.Render("\n  error: "+m.err.Error()) +
			"\n\n" + styleMuted.Render("  q for exit")

	case phaseList:
		return "\n" + renderList(m)

	case phaseDetail, phaseConnecting, phaseConnected:
		return "\n" + renderDetail(m)
	}

	return ""
}
