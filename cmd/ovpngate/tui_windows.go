//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/gdamore/tcell/v2"
	"github.com/kurojs/ovpngate/internal/connect"
	"github.com/kurojs/ovpngate/internal/favstore"
	"github.com/kurojs/ovpngate/internal/vpngate"
)

const (
	tuiHeaderRows = 3
	tuiFooterRows = 3

	tuiColorText     = "#F3F6F9"
	tuiColorMuted    = "#5C6170"
	tuiColorPurpleLt = "#9C9CD4"
	tuiColorPurple   = "#8080C0"
	tuiColorGreen    = "#86EFAC"
	tuiColorWarning  = "#FCD34D"
	tuiColorError    = "#FCA5A5"
	tuiColorBorder   = "#1E293B"
)

type tuiModel struct {
	all      []vpngate.Server
	sorted   []vpngate.Server
	filtered []vpngate.Server
	rowIdx   []int
	cursor   int
	scroll   int
	viewH    int
	loading  bool
	loadErr  error
	status   string
	detail   *vpngate.Server

	filter           string
	filterCountry    string
	filterCountries  []string
	filterCountryIdx int

	connecting bool
	connected  *vpngate.Server
	assignedIP string
	connErr    error

	favStore         *favstore.Store
	offlineFavorites []vpngate.Server
	offlineSet       map[string]bool
}

type connResult struct {
	cancelled bool
	ip        string
	err       error
}

func newTUIModel() *tuiModel {
	return &tuiModel{cursor: 0, scroll: 0, loading: true, filter: "all", offlineSet: make(map[string]bool)}
}

func sampleServers() []vpngate.Server {
	return []vpngate.Server{
		{HostName: "public-vpn-214", IP: "185.170.196.133", CountryShort: "JP", CountryLong: "Japan", Ping: 15, Speed: 2524, Sessions: 116},
		{HostName: "public-vpn-36", IP: "202.53.7.186", CountryShort: "JP", CountryLong: "Japan", Ping: 120, Speed: 827, Sessions: 95},
		{HostName: "public-vpn-72", IP: "223.165.7.99", CountryShort: "JP", CountryLong: "Japan", Ping: 35, Speed: 624, Sessions: 44},
		{HostName: "public-vpn-9", IP: "185.170.194.68", CountryShort: "JP", CountryLong: "Japan", Ping: 18, Speed: 1502, Sessions: 102},
		{HostName: "public-vpn-152", IP: "185.220.44.10", CountryShort: "JP", CountryLong: "Japan", Ping: 67, Speed: 352, Sessions: 41},
	}
}

func (m *tuiModel) setServers(s []vpngate.Server) {
	m.all = s
	m.sorted = make([]vpngate.Server, len(s))
	copy(m.sorted, s)
	sort.Slice(m.sorted, func(i, j int) bool {
		return m.sorted[i].Speed > m.sorted[j].Speed
	})
	m.loading = false
	if len(m.sorted) == 0 {
		m.status = "no servers yet"
		m.filtered = nil
		m.rowIdx = nil
		m.cursor = 0
		m.scroll = 0
		return
	}
	m.buildCountryList()
	m.detectOfflineFavorites()
	m.applyFilter()
	m.status = fmt.Sprintf("%d servers  sorted by speed", len(m.sorted))
}

func (m *tuiModel) buildCountryList() {
	m.filterCountries = nil
	seen := make(map[string]struct{})
	for _, s := range m.sorted {
		if s.CountryShort != "" {
			if _, ok := seen[s.CountryShort]; !ok {
				seen[s.CountryShort] = struct{}{}
				m.filterCountries = append(m.filterCountries, s.CountryShort)
			}
		}
	}
	sort.Strings(m.filterCountries)
}

func (m *tuiModel) detectOfflineFavorites() {
	m.offlineFavorites = nil
	m.offlineSet = make(map[string]bool)
	if m.favStore == nil {
		return
	}
	online := make(map[string]bool)
	for _, s := range m.all {
		online[s.IP] = true
	}
	for _, f := range m.favStore.GetAll() {
		if !online[f.IP] {
			m.offlineFavorites = append(m.offlineFavorites, vpngate.Server{
				HostName:     f.HostName,
				IP:           f.IP,
				CountryShort: f.CountryShort,
				CountryLong:  f.CountryLong,
			})
			m.offlineSet[f.IP] = true
		}
	}
}

func (m *tuiModel) applyFilter() {
	m.filtered = make([]vpngate.Server, 0, len(m.sorted))
	for _, s := range m.sorted {
		if m.filterCountry != "" && s.CountryShort != m.filterCountry {
			continue
		}
		if m.filter == "fav" && !m.isFav(s.IP) {
			continue
		}
		m.filtered = append(m.filtered, s)
	}
	if m.filter == "fast" {
		sort.Slice(m.filtered, func(i, j int) bool {
			return m.filtered[i].Speed > m.filtered[j].Speed
		})
	}
	if m.filter == "fav" {
		m.filtered = append(m.filtered, m.offlineFavorites...)
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
	m.rebuildRowIdx()
	m.scroll = 0
	if m.viewH > 0 {
		m.scrollClamp(m.viewH)
	}
}

func (m *tuiModel) rebuildRowIdx() {
	m.rowIdx = make([]int, len(m.filtered))
	row := 0
	prev := ""
	for i := range m.filtered {
		if m.filtered[i].CountryShort != prev {
			row++
			prev = m.filtered[i].CountryShort
		}
		m.rowIdx[i] = row
		row++
	}
}

func (m *tuiModel) isFav(ip string) bool {
	return m.favStore != nil && m.favStore.IsFavorite(ip)
}

func (m *tuiModel) toggleFavorite(s vpngate.Server) {
	if m.favStore == nil {
		return
	}
	if m.favStore.IsFavorite(s.IP) {
		m.favStore.Remove(s.IP)
	} else {
		m.favStore.Add(s.IP, s.HostName, s.CountryShort, s.CountryLong)
	}
	m.detectOfflineFavorites()
	m.applyFilter()
}

func (m *tuiModel) cycleCountry() {
	m.filterCountryIdx++
	if m.filterCountryIdx > len(m.filterCountries) {
		m.filterCountryIdx = 0
	}
	m.filterCountry = ""
	if m.filterCountryIdx > 0 {
		m.filterCountry = m.filterCountries[m.filterCountryIdx-1]
	}
	if m.filter == "fav" {
		m.filter = "all"
	}
	m.applyFilter()
}

func (m *tuiModel) countryStart(cursor int) int {
	if cursor <= 0 || len(m.filtered) == 0 {
		return 0
	}
	c := m.filtered[cursor].CountryShort
	i := cursor
	for i > 0 && m.filtered[i-1].CountryShort == c {
		i--
	}
	return i
}

func (m *tuiModel) scrollClamp(h int) {
	n := len(m.filtered)
	if n == 0 {
		m.scroll = 0
		return
	}
	rows := h - tuiHeaderRows - tuiFooterRows
	if rows < 1 {
		rows = 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}

	cr := m.rowIdx[m.cursor]
	if cr < m.scroll {
		m.scroll = cr
		start := m.countryStart(m.cursor)
		if start >= 0 {
			m.scroll = m.rowIdx[start] - 1
		}
	}
	if cr >= m.scroll+rows {
		m.scroll = cr - rows + 1
	}
	total := m.rowIdx[n-1] + 1
	maxScroll := total - rows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m *tuiModel) moveCursor(delta, h int) {
	n := len(m.filtered)
	if n == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	m.scrollClamp(h)
}

func clampPing(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func emitStr(scr tcell.Screen, x, y int, s string, st tcell.Style, w int) {
	if w <= 0 {
		return
	}
	runes := []rune(s)
	for i := 0; i < len(runes) && i < w; i++ {
		scr.SetContent(x+i, y, runes[i], nil, st)
	}
}

func (m *tuiModel) resolveConn(res connResult) {
	m.connecting = false
	switch {
	case res.cancelled:
		m.connErr = nil
	case res.err != nil:
		m.connErr = res.err
	default:
		m.connected = m.detail
		m.assignedIP = res.ip
		m.connErr = nil
	}
}

func drawTUI(scr tcell.Screen, m *tuiModel) {
	scr.Clear()
	w, h := scr.Size()
	if h <= 0 {
		return
	}
	m.viewH = h

	base := tcell.StyleDefault.Foreground(tcell.GetColor(tuiColorText))
	purple := base.Foreground(tcell.GetColor(tuiColorPurple))
	purpleLt := base.Foreground(tcell.GetColor(tuiColorPurpleLt))
	muted := base.Foreground(tcell.GetColor(tuiColorMuted))
	green := base.Foreground(tcell.GetColor(tuiColorGreen))
	yellow := base.Foreground(tcell.GetColor(tuiColorWarning))
	errSt := base.Foreground(tcell.GetColor(tuiColorError))
	sel := base.Background(tcell.GetColor("#3D6B52"))

	if m.detail != nil {
		d := m.detail
		emitStr(scr, 0, 0, "ovpngate  ["+d.CountryShort+"]  "+d.CountryLong, purple, w)
		emitStr(scr, 0, 1, "host      "+d.HostName, muted, w)
		emitStr(scr, 0, 2, "ip        "+d.IP, muted, w)
		emitStr(scr, 0, 3, "ping      "+fmt.Sprintf("%d ms", clampPing(d.Ping)), muted, w)
		emitStr(scr, 0, 4, "speed     "+fmt.Sprintf("%d KB/s", d.Speed), muted, w)
		emitStr(scr, 0, 5, "sessions  "+fmt.Sprintf("%d", d.Sessions), muted, w)
		if m.isFav(d.IP) {
			emitStr(scr, 0, 6, "* favorite", green, w)
		}

		isActive := m.connected != nil && m.connected.HostName == d.HostName
		status := ""
		statusStyle := green
		hints := ""
		switch {
		case m.connecting:
			status = "connecting...  (esc to cancel)"
			statusStyle = yellow
			hints = "esc cancel   q quit"
		case isActive:
			status = "connected  ip " + m.assignedIP
			hints = "d disconnect   q quit"
		case m.connErr != nil:
			status = "error: " + m.connErr.Error()
			statusStyle = errSt
			hints = "enter retry   esc back   q quit"
		case len(d.OvpnConfig) == 0:
			status = "unavailable  (offline server, no ovpn config)"
			statusStyle = muted
			hints = "s star   esc back   q quit"
		default:
			status = "ready  press enter to connect"
			hints = "s star  enter connect  esc back  q quit"
		}

		y := 8
		if d.Message != "" {
			emitStr(scr, 0, y, "message  "+d.Message, purpleLt, w)
			emitStr(scr, 0, y+1, "", base, w)
			y = y + 2
		}
		emitStr(scr, 0, y, status, statusStyle, w)
		emitStr(scr, 0, y+1, hints, muted, w)
		return
	}

	header := "ovpngate"
	if m.filterCountry != "" {
		header += "  [" + m.filterCountry + "]"
	} else {
		header += "  all"
	}
	if m.filter == "fast" {
		header += "  fast"
	}
	if m.filter == "fav" {
		header += "  fav"
		if n := len(m.offlineFavorites); n > 0 {
			header += fmt.Sprintf(" (%d offline)", n)
		}
	}
	header += fmt.Sprintf("  (%d)", len(m.filtered))
	emitStr(scr, 0, 0, header, purpleLt, w)
	emitStr(scr, 0, 1, "r ref  a all  f fast  v fav  c country  s star  enter  q quit", muted, w)

	if m.loading {
		emitStr(scr, 0, tuiHeaderRows, "fetching...", green, w)
		return
	}
	if m.loadErr != nil {
		emitStr(scr, 0, tuiHeaderRows, "error: "+m.loadErr.Error(), errSt, w)
		return
	}
	if len(m.filtered) == 0 {
		msg := "no servers found"
		if m.filter == "fav" {
			msg = "no favorites yet — press s to star a server"
		}
		emitStr(scr, 0, tuiHeaderRows, msg, muted, w)
		return
	}

	prevCountry := ""
	idx := 0
	for i := 0; i < len(m.filtered); i++ {
		s := m.filtered[i]
		if s.CountryShort != prevCountry {
			rowY := tuiHeaderRows + idx - m.scroll
			if rowY >= tuiHeaderRows && rowY < h-tuiFooterRows {
				emitStr(scr, 1, rowY, "["+s.CountryShort+"]  "+s.CountryLong, purpleLt, w)
			}
			prevCountry = s.CountryShort
			idx++
		}
		rowY := tuiHeaderRows + idx - m.scroll
		if rowY >= tuiHeaderRows && rowY < h-tuiFooterRows {
			st := base
			if i == m.cursor {
				st = sel
			}
			star := "  "
			if m.isFav(s.IP) {
				star = "* "
			}
			line := fmt.Sprintf("%s%-15s  %-15s  %3d ms  %5d KB/s  %3d",
				star, s.HostName, s.IP, clampPing(s.Ping), s.Speed, s.Sessions)
			emitStr(scr, 2, rowY, line, st, w)
		}
		idx++
	}

	emitStr(scr, 0, h-tuiFooterRows, m.status, muted, w)
	pos := ""
	if len(m.filtered) > 0 {
		rows := h - tuiHeaderRows - tuiFooterRows
		if rows < 1 {
			rows = 1
		}
		first, last := -1, -1
		for i, r := range m.rowIdx {
			if r >= m.scroll && r < m.scroll+rows {
				if first == -1 {
					first = i
				}
				last = i
			}
		}
		if first != -1 {
			pos = fmt.Sprintf("%d-%d of %d", first+1, last+1, len(m.filtered))
		}
	}
	emitStr(scr, 0, h-tuiFooterRows+1, pos, muted, w)
}

func startConnect(m *tuiModel, resCh chan connResult) {
	if m.detail == nil {
		return
	}
	d := m.detail
	go func() {
		if _, cerr := connect.Connect(d.HostName, d.OvpnConfig); cerr != nil {
			resCh <- connResult{err: cerr}
			return
		}
		ip, werr := connect.WaitForTunnel()
		if errors.Is(werr, connect.ErrCancelConnect) {
			resCh <- connResult{cancelled: true}
			return
		}
		resCh <- connResult{ip: ip, err: werr}
	}()
}

func runTUI() int {
	m := newTUIModel()

	if favPath, err := favstore.DefaultPath(); err == nil {
		store := favstore.New(favPath)
		if err := store.Load(); err == nil {
			m.favStore = store
		}
	}
	defer func() {
		if m.favStore != nil {
			_ = m.favStore.Save()
		}
	}()

	servers, ferr := vpngate.Fetch()
	if ferr != nil {
		m.loadErr = ferr
		m.setServers(sampleServers())
	} else {
		m.setServers(servers)
	}

	scr, err := tcell.NewScreen()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ovpngate:", err)
		return 1
	}
	if err := scr.Init(); err != nil {
		fmt.Fprintln(os.Stderr, "ovpngate:", err)
		return 1
	}
	defer scr.Fini()

	evCh := make(chan tcell.Event, 32)
	evQuit := make(chan struct{})
	defer close(evQuit)
	resCh := make(chan connResult, 1)
	go scr.ChannelEvents(evCh, evQuit)

	draw := func() {
		drawTUI(scr, m)
		scr.Show()
	}

	for {
		_, h := scr.Size()
		select {
		case res := <-resCh:
			m.resolveConn(res)
			draw()
		case ev, ok := <-evCh:
			if !ok {

				connect.Disconnect()
				return 0
			}
			switch e := ev.(type) {
			case *tcell.EventResize:
				draw()
			case *tcell.EventKey:
				switch e.Key() {
				case tcell.KeyCtrlC, tcell.KeyEscape:
					if m.detail != nil {
						if m.connecting {
							connect.Cancel()
							m.connecting = false
							draw()
						} else if m.connected != nil && m.connected.HostName == m.detail.HostName {

						} else {
							m.connErr = nil
							m.detail = nil
							draw()
						}
					} else {
						connect.Disconnect()
						return 0
					}
				case tcell.KeyEnter:
					if m.detail != nil {
						if m.connecting {

						} else if m.connected != nil && m.connected.HostName == m.detail.HostName {

					} else if len(m.detail.OvpnConfig) == 0 {
						m.connErr = errors.New("offline server, no ovpn config")
						draw()
						} else {
							m.connecting = true
							m.connErr = nil
							draw()
							startConnect(m, resCh)
						}
					} else if len(m.filtered) > 0 {
						m.connErr = nil
						m.detail = &m.filtered[m.cursor]
						draw()
					}
				case tcell.KeyUp:
					if m.detail == nil {
						m.moveCursor(-1, h)
						draw()
					}
				case tcell.KeyDown:
					if m.detail == nil {
						m.moveCursor(1, h)
						draw()
					}
				case tcell.KeyPgUp:
					if m.detail == nil {
						m.pageMove(-(h - tuiHeaderRows - tuiFooterRows), h)
						draw()
					}
				case tcell.KeyPgDn:
					if m.detail == nil {
						m.pageMove(h-tuiHeaderRows-tuiFooterRows, h)
						draw()
					}
				default:
					switch r := e.Rune(); r {
					case 'q', 'Q':
						if m.detail != nil {
							if m.connecting {
								connect.Cancel()
								m.connecting = false
								draw()
							} else {
								connect.Disconnect()
								return 0
							}
						} else {
							connect.Disconnect()
							return 0
						}
					case 'a', 'A':
						if m.detail == nil {
							m.filter = "all"
							m.filterCountry = ""
							m.filterCountryIdx = 0
							m.applyFilter()
							draw()
						}
					case 'f', 'F':
						if m.detail == nil {
							m.filter = "fast"
							m.applyFilter()
							draw()
						}
					case 'v', 'V':
						if m.detail == nil {
							m.filter = "fav"
							m.applyFilter()
							draw()
						}
					case 'c', 'C':
						if m.detail == nil {
							m.cycleCountry()
							draw()
						}
					case 's', 'S':
						if m.favStore != nil {
							if m.detail != nil {
								if m.connected != nil && m.connected.HostName == m.detail.HostName {

								} else {
									m.toggleFavorite(*m.detail)
									draw()
								}
							} else if len(m.filtered) > 0 {
								m.toggleFavorite(m.filtered[m.cursor])
								draw()
							}
						}
					case 'd', 'D':
						if m.detail != nil && m.connected != nil && m.connected.HostName == m.detail.HostName {
							_ = connect.Disconnect()
							m.connected = nil
							m.assignedIP = ""
							m.connErr = nil
							m.detail = nil
							draw()
						}
					case 'j', 'J':
						if m.detail == nil {
							m.moveCursor(1, h)
							draw()
						}
					case 'k', 'K':
						if m.detail == nil {
							m.moveCursor(-1, h)
							draw()
						}
					case 'r', 'R':
						if m.detail == nil {
							servers, ferr := vpngate.Fetch()
							if ferr != nil {
								m.loadErr = ferr
								m.setServers(sampleServers())
							} else {
								m.setServers(servers)
							}
							draw()
						}
					}
				}
			}
		}
	}
}

func (m *tuiModel) pageMove(step, h int) {
	n := len(m.filtered)
	if n == 0 {
		return
	}
	m.cursor += step
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	m.scrollClamp(h)
}
