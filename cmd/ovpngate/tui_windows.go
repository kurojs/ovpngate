//go:build windows

package main

// ovpngate Windows: interfaz identica en funcionalidad a mac/linux
// (palette lipgloss de internal/ui/styles.go, favoritos persistentes via
// internal/favstore, filtros all/fast/fav/pais, conexion real via
// internal/connect con OpenVPN + helper elevado). Lista agrupada por paises
// con headers morados, ping clampeado 0..100ms. Fetch real con fallback
// determinista offline (mismos 5 servers de muestra) para que la UI sea
// demostrable sin red.
//
// DECISION: la conexion bloquea hasta 30s (WaitForTunnel), asi que corre en
// una goroutine y el resultado vuelve por un channel. El loop principal usa
// tcell ChannelEvents + select para no bloquear la UI mientras conecta.

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

	// Paleta identica a internal/ui/styles.go (lipgloss). Cero invento.
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
	sorted   []vpngate.Server // por velocidad desc (contrato de tests)
	filtered []vpngate.Server // tras aplicar filtros
	rowIdx   []int            // fila de pantalla de cada server (headers de pais incluidos)
	cursor   int
	scroll   int
	loading  bool
	loadErr  error
	status   string
	detail   *vpngate.Server // Enter abre la seccion de conexion/detalles
	logs     []string

	filter           string   // all | fast | fav
	filterCountry    string   // "" = todos, o codigo de pais (JP...)
	filterCountries  []string // paises disponibles, ordenados
	filterCountryIdx int

	connecting bool            // true = conectando al ovpn
	connected  *vpngate.Server // server conectado, si hay
	assignedIP string          // IP asignada por el tunel
	connErr    error           // ultimo error de conexion

	favStore         *favstore.Store
	offlineFavorites []vpngate.Server
	offlineSet       map[string]bool
}

// connResult es lo que la goroutine de conexion devuelve al loop principal.
type connResult struct {
	cancelled bool
	ip        string
	err       error
}

func newTUIModel() *tuiModel {
	return &tuiModel{cursor: 0, scroll: 0, loading: true, filter: "all", offlineSet: make(map[string]bool)}
}

// sampleServers devuelve datos deterministas (5 servers, sin mojibake),
// igual contrato que los tests esperan (deterministico, len 5, max 2524).
// No traen OvpnConfig: son la muestra offline, la UI los marca como
// "unavailable" y no permite conectar.
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

// detectOfflineFavorites marca los favoritos guardados que ya no estan en la
// lista online: se siguen mostrando (en el filtro fav) para poder
// desmarcarlos o verlos, con datos minimos.
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
	m.scrollClamp(m.viewportRows())
}

// rebuildRowIdx calcula la fila de pantalla de cada server: los headers de
// pais ocupan su propia fila (igual que en el dibujo), asi que desfasan el
// indice de pantalla respecto del indice de la lista.
func (m *tuiModel) rebuildRowIdx() {
	m.rowIdx = make([]int, len(m.filtered))
	row := 0
	prev := ""
	for i := range m.filtered {
		if m.filtered[i].CountryShort != prev {
			row++ // fila del header de pais
			prev = m.filtered[i].CountryShort
		}
		m.rowIdx[i] = row
		row++ // fila del server
	}
}

// isFav consulta el store de favoritos (nulable en tests).
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

// cycleCountry avanza el filtro por paises ("" -> JP -> ... -> "").
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

func (m *tuiModel) viewportRows() int {
	return tuiHeaderRows + tuiFooterRows + 1 // altura minima segura para clamp
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
	// La visibilidad se mide en filas de pantalla REALES (rowIdx incluye los
	// headers de pais). Sin esto el cursor en la fila de abajo quedaba mas
	// alla del borde visible: scrollClamp contaba servers, el dibujo filas.
	cr := m.rowIdx[m.cursor]
	if cr < m.scroll {
		m.scroll = cr
	}
	if cr >= m.scroll+rows {
		m.scroll = cr - rows + 1
	}
	total := m.rowIdx[n-1] + 1 // filas de pantalla totales (headers + servers)
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

// clampPing acota el ping a 0..100ms (promedio, como la seccion Linux).
func clampPing(p int) int {
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// emitStr pinta una linea ASCII acotada al ancho w (sin mojibake).
func emitStr(scr tcell.Screen, x, y int, s string, st tcell.Style, w int) {
	if w <= 0 {
		return
	}
	runes := []rune(s)
	for i := 0; i < len(runes) && i < w; i++ {
		scr.SetContent(x+i, y, runes[i], nil, st)
	}
}

// resolveConn aplica el resultado de la goroutine de conexion al modelo.
// Es una funcion pura para poder testearla headless.
func (m *tuiModel) resolveConn(res connResult) {
	m.connecting = false
	switch {
	case res.cancelled:
		m.connErr = nil
		m.logs = appendLog(m.logs, "cancelled")
	case res.err != nil:
		m.connErr = res.err
		m.logs = appendLog(m.logs, "error: "+res.err.Error())
	default:
		m.connected = m.detail
		m.assignedIP = res.ip
		m.connErr = nil
		m.logs = appendLog(m.logs, "connected with IP: "+res.ip)
	}
}

// appendLog mantiene un buffer acotado de logs (ultimas 6 lineas).
func appendLog(logs []string, line string) []string {
	logs = append(logs, line)
	if len(logs) > 6 {
		logs = logs[len(logs)-6:]
	}
	return logs
}

// drawTUI pinta list + seccion de conexion/detalles (Enter) + agrupacion por
// paises, con la misma palette que Linux. El cursor navega servers; los
// headers de pais se insertan como filas moradas.
func drawTUI(scr tcell.Screen, m *tuiModel) {
	scr.Clear()
	w, h := scr.Size()
	if h <= 0 {
		return
	}

	base := tcell.StyleDefault.Foreground(tcell.GetColor(tuiColorText))
	purple := base.Foreground(tcell.GetColor(tuiColorPurple))
	purpleLt := base.Foreground(tcell.GetColor(tuiColorPurpleLt))
	muted := base.Foreground(tcell.GetColor(tuiColorMuted))
	green := base.Foreground(tcell.GetColor(tuiColorGreen))
	yellow := base.Foreground(tcell.GetColor(tuiColorWarning))
	errSt := base.Foreground(tcell.GetColor(tuiColorError))
	inv := base.Reverse(true)

	// Seccion de conexion/detalles (Enter): igual que Linux.
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
		if len(m.logs) > 0 {
			ly := y + 2
			for _, l := range m.logs {
				ls := base
				if len(l) >= 6 && l[:6] == "error:" {
					ls = errSt
				}
				emitStr(scr, 0, ly, l, ls, w)
				ly++
				if ly > h-1 {
					break
				}
			}
		}
		return
	}

	// Header: titulo + filtros activos + teclas (la unica linea de atajos).
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

	// Lista agrupada por paises: header morado antes del primer server de un
	// nuevo pais (mecanismo identico a la seccion mac/linux). El contador idx
	// avanza siempre (visible o no) para que el scroll sea consistente.
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
				st = inv
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

	// Footer: status + posicion del scroll (rango de servers visibles).
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

// startConnect lanza la conexion real en background (OpenVPN via helper
// elevado). El resultado va por resCh; nunca toca la pantalla desde aca.
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

// runTUI es el loop principal de Windows. Devuelve codigo de salida.
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

	// La conexion corre en background: solo un canal para sus resultados y
	// los eventos del usuario entrando por ChannelEvents -> select evita que
	// una conexion de 30s congele la interfaz.
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
				// El screen se cerro (Fini): terminamos.
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
							m.logs = appendLog(m.logs, "cancelled")
							draw()
						} else if m.connected != nil && m.connected.HostName == m.detail.HostName {
							// Conectado: esc no desconecta, d lo hace.
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
							// Ya conectando: ignorar repetidos.
						} else if m.connected != nil && m.connected.HostName == m.detail.HostName {
							// Ya conectado a este server: no reconectar.
						} else if len(m.detail.OvpnConfig) == 0 {
							m.connErr = errors.New("offline server, no ovpn config")
							m.logs = appendLog(m.logs, "error: offline server, no ovpn config")
							draw()
						} else {
							m.connecting = true
							m.connErr = nil
							m.logs = appendLog(m.logs, "connecting...")
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
								m.logs = appendLog(m.logs, "cancelled")
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
									// No tocar favorito del server conectado.
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
							m.logs = appendLog(m.logs, "disconnected")
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
