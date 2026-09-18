//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kurojs/ovpngate/internal/favstore"
	"github.com/kurojs/ovpngate/internal/vpngate"
)

func newFavModel(t *testing.T) *tuiModel {
	t.Helper()
	m := newTUIModel()
	m.favStore = favstore.New(filepath.Join(t.TempDir(), "fav.json"))
	m.setServers(sampleServers())
	return m
}

func TestTUIApplyFilterFast(t *testing.T) {
	m := newFavModel(t)
	m.filter = "fast"
	m.applyFilter()
	if len(m.filtered) != 5 {
		t.Fatalf("fast: filtered=%d quiero 5", len(m.filtered))
	}
	for i := 1; i < len(m.filtered); i++ {
		if m.filtered[i-1].Speed < m.filtered[i].Speed {
			t.Fatalf("fast desordenado en %d", i)
		}
	}
}

func TestTUIApplyFilterFavWithOffline(t *testing.T) {
	m := newFavModel(t)
	m.favStore.Add("185.170.196.133", "public-vpn-214", "JP", "Japan")
	m.favStore.Add("9.9.9.9", "dead-host", "US", "United States")
	m.detectOfflineFavorites()
	m.filter = "fav"
	m.applyFilter()
	if len(m.filtered) != 2 {
		t.Fatalf("fav: filtered=%d quiero 2 (1 online + 1 offline)", len(m.filtered))
	}
	if !m.isFav(m.filtered[0].IP) && !m.isFav(m.filtered[1].IP) {
		t.Fatalf("fav deberia contener solo favoritos")
	}
	if len(m.offlineFavorites) != 1 || m.offlineFavorites[0].IP != "9.9.9.9" {
		t.Fatalf("offlineFavorites inesperado: %+v", m.offlineFavorites)
	}
}

func TestTUIToggleFavoriteAddRemove(t *testing.T) {
	m := newFavModel(t)
	s := m.sorted[0]
	m.toggleFavorite(s)
	if !m.isFav(s.IP) {
		t.Fatalf("toggle no agrego favorito %s", s.IP)
	}
	m.toggleFavorite(s)
	if m.isFav(s.IP) {
		t.Fatalf("toggle no removio favorito %s", s.IP)
	}
}

func TestTUIDetectOfflineFavorites(t *testing.T) {
	m := newFavModel(t)
	m.favStore.Add("10.0.0.1", "gone", "AR", "Argentina")
	m.detectOfflineFavorites()
	if len(m.offlineFavorites) != 1 {
		t.Fatalf("offlineFavorites=%d quiero 1", len(m.offlineFavorites))
	}
	if !m.offlineSet["10.0.0.1"] {
		t.Fatalf("offlineSet no marco la IP fantasma")
	}
}

func TestTUICycleCountry(t *testing.T) {
	m := newFavModel(t)
	// sample todos JP: ciclo 0 -> JP -> "" -> JP
	m.cycleCountry()
	if m.filterCountry != "JP" {
		t.Fatalf("ciclo 1: filterCountry=%q quiero JP", m.filterCountry)
	}
	m.cycleCountry()
	if m.filterCountry != "" {
		t.Fatalf("ciclo 2: filterCountry=%q quiero vacio", m.filterCountry)
	}
	if len(m.filtered) != 5 {
		t.Fatalf("sin filtro de pais: filtered=%d quiero 5", len(m.filtered))
	}
}

func TestTUIPageMove(t *testing.T) {
	m := newFavModel(t)
	m.setServers(makeBigList(50))
	m.cursor = 0
	m.pageMove(18, 24)
	if m.cursor != 18 {
		t.Fatalf("pgdn: cursor=%d quiero 18", m.cursor)
	}
	m.pageMove(-18, 24)
	if m.cursor != 0 {
		t.Fatalf("pgup: cursor=%d quiero 0", m.cursor)
	}
}

func makeMixedList(n int) []vpngate.Server {
	out := make([]vpngate.Server, n)
	cs := []string{"JP", "US", "BR"}
	cl := []string{"Japan", "United States", "Brazil"}
	for i := 0; i < n; i++ {
		out[i] = vpngate.Server{
			HostName:     "public-vpn-mixed",
			IP:           fmt.Sprintf("10.0.%d.%d", i%250, i),
			CountryShort: cs[i%3],
			CountryLong:  cl[i%3],
			Ping:         i % 150,
			Speed:        (i * 11) % 3000,
			Sessions:     i,
		}
	}
	return out
}

// Regression del selector perdido: con grupos de pais intercalados cada
// header consume una fila de pantalla, de modo que el indice de pantalla del
// cursor (rowIdx) desfasa del indice de lista. scrollClamp debe mantenerlo
// SIEMPRE dentro de la ventana, en cualquier posicion del cursor.
func TestTUIScrollVisibleWithCountryHeaders(t *testing.T) {
	m := newFavModel(t)
	m.setServers(makeMixedList(30))
	h := 24
	rows := h - tuiHeaderRows - tuiFooterRows
	for i := len(m.filtered) - 1; i >= 0; i-- {
		m.cursor = i
		m.scroll = 0
		m.scrollClamp(h)
		cr := m.rowIdx[m.cursor]
		if cr < m.scroll || cr >= m.scroll+rows {
			t.Fatalf("cursor %d (fila %d) fuera de ventana [%d,%d)", i, cr, m.scroll, m.scroll+rows)
		}
	}
}

func TestTUIFilteredPointersStable(t *testing.T) {
	m := newFavModel(t)
	m.filter = "fav"
	m.applyFilter()
	if len(m.filtered) != 0 {
		t.Fatalf("sin favoritos filtrado deberia ser vacio, tengo %d", len(m.filtered))
	}
	m.detail = nil
	if got := reflect.DeepEqual(m.filtered, []vpngate.Server{}); !got {
		t.Fatalf("filtered no refleja union vacia")
	}
}
