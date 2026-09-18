//go:build windows

package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kurojs/ovpngate/internal/vpngate"
)

// The Windows TUI's core contract is a CORRECT list interface: sorted rows, a
// clamped cursor, and a scroll state that ALWAYS keeps the inverted cursor row
// visible, plus a pure connection resolver.  None of that needs a screen — it's
// pure data, so it's provable headless with table tests.  (Name collisions
// with bubbletea's model tests on mac/linux are impossible: this file is
// windows-tagged.)

func TestTUISampleDeterministic(t *testing.T) {
	a := sampleServers()
	b := sampleServers()
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("sampleServers() no es determinista")
	}
	if len(a) != 5 {
		t.Fatalf("sampleServers(): quiero 5 servers, tengo %d", len(a))
	}
	for _, s := range a {
		if s.HostName == "" || s.IP == "" {
			t.Fatalf("server con campos vacios: %+v", s)
		}
	}
}

func TestTUISetServersSortsBySpeedDesc(t *testing.T) {
	m := newTUIModel()
	m.setServers(sampleServers())
	if len(m.sorted) != 5 {
		t.Fatalf("esperaba 5 sorted, tengo %d", len(m.sorted))
	}
	for i := 1; i < len(m.sorted); i++ {
		if m.sorted[i-1].Speed < m.sorted[i].Speed {
			t.Fatalf("orden roto en %d: %d < %d", i, m.sorted[i-1].Speed, m.sorted[i].Speed)
		}
	}
	if m.sorted[0].Speed != 2524 {
		t.Fatalf("max speed deberia ser 2524, tengo %d", m.sorted[0].Speed)
	}
}

func TestTUIMoveCursorClamps(t *testing.T) {
	m := newTUIModel()
	m.setServers(sampleServers())
	m.moveCursor(999, 24)
	if m.cursor != len(m.sorted)-1 {
		t.Fatalf("moveCursor(+999): cursor=%d quiero %d", m.cursor, len(m.sorted)-1)
	}
	m.moveCursor(-999, 24)
	if m.cursor != 0 {
		t.Fatalf("moveCursor(-999): cursor=%d quiero 0", m.cursor)
	}
}

func TestTUIScrollClampKeepsCursorVisible(t *testing.T) {
	m := newTUIModel()
	m.setServers(sampleServers())
	// head 3 + foot 3 => con h=24 hay 18 filas de lista; 5 servers caben.
	h := 24
	m.cursor = 4
	m.scroll = 0
	m.scrollClamp(h)
	if m.cursor < m.scroll || m.cursor >= m.scroll+18 {
		t.Fatalf("cursor %d fuera de ventana [%d,%d)", m.cursor, m.scroll, m.scroll+18)
	}

	// Lista más larga que la pantalla: forzar scroll hacia abajo y verificar
	// que el cursor sigue visible tras mover.  makeBigList es 100% JP, asi que
	// hay UN header de pais: el scroll se cuenta en filas de pantalla, no en
	// servers (rowIdx[49]=50), dando scroll 33, no 32 (50-18+1).
	m.setServers(makeBigList(50))
	m.cursor = 49
	m.scroll = 0
	m.scrollClamp(h)
	if m.cursor < m.scroll || m.cursor >= m.scroll+18 {
		t.Fatalf("cursor %d fuera de ventana [%d,%d)", m.cursor, m.scroll, m.scroll+18)
	}
	if m.scroll != 33 {
		t.Fatalf("scroll deberia ser 33 (=50-18+1 con header), tengo %d", m.scroll)
	}
}

func TestTUICursorVisibleAfterShrink(t *testing.T) {
	m := newTUIModel()
	m.setServers(makeBigList(10))
	m.cursor = 9
	m.setServers(sampleServers()) // 10 -> 5
	if m.cursor >= len(m.sorted) {
		t.Fatalf("cursor %d no clampado tras encoger a %d", m.cursor, len(m.sorted))
	}
}

func TestTUIResolveConnSuccess(t *testing.T) {
	m := newTUIModel()
	m.setServers(sampleServers())
	m.detail = &m.sorted[0]
	m.resolveConn(connResult{ip: "10.8.0.2"})
	if m.connecting {
		t.Fatalf("resolveConn success deberia apagar connecting")
	}
	if m.connected == nil || m.connected.HostName != m.detail.HostName {
		t.Fatalf("resolveConn success no marco connected: %+v", m.connected)
	}
	if m.assignedIP != "10.8.0.2" {
		t.Fatalf("assignedIP=%q quiero 10.8.0.2", m.assignedIP)
	}
	if m.connErr != nil {
		t.Fatalf("connErr deberia ser nil, tengo %v", m.connErr)
	}
}

func TestTUIResolveConnError(t *testing.T) {
	m := newTUIModel()
	m.setServers(sampleServers())
	m.detail = &m.sorted[0]
	m.connecting = true
	m.resolveConn(connResult{err: errors.New("openvpn not found")})
	if m.connecting {
		t.Fatalf("resolveConn error deberia apagar connecting")
	}
	if m.connected != nil {
		t.Fatalf("resolveConn error no deberia conectar: %v", m.connected)
	}
	if m.connErr == nil {
		t.Fatalf("connErr deberia guardar el error")
	}
}

func TestTUIResolveConnCancelled(t *testing.T) {
	m := newTUIModel()
	m.setServers(sampleServers())
	m.detail = &m.sorted[0]
	m.connecting = true
	m.resolveConn(connResult{cancelled: true})
	if m.connecting {
		t.Fatalf("resolveConn cancelled deberia apagar connecting")
	}
	if m.connected != nil {
		t.Fatalf("resolveConn cancelled no deberia conectar: %v", m.connected)
	}
	if m.connErr != nil {
		t.Fatalf("cancel no es error, connErr=%v", m.connErr)
	}
}

func makeBigList(n int) []vpngate.Server {
	out := make([]vpngate.Server, n)
	for i := 0; i < n; i++ {
		out[i] = vpngate.Server{
			HostName:     "public-vpn-big",
			IP:           "1.2.3.4",
			CountryShort: "JP",
			CountryLong:  "Japan",
			Ping:         i % 200,
			Speed:        (i * 7) % 3000,
			Sessions:     i,
		}
	}
	return out
}
