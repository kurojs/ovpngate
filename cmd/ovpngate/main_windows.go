//go:build windows

package main

import (
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kurojs/ovpngate/internal/connect"
	"github.com/kurojs/ovpngate/internal/favstore"
	"github.com/kurojs/ovpngate/internal/ui"
)

func main() {
	// Elevated helper mode: launched via "runas" by startOpenVPN when the
	// user connects.  It runs OpenVPN with the privileges the TUN driver
	// requires, then takes its orders from the workdir control files.
	if len(os.Args) >= 4 && os.Args[1] == "--helper" {
		workdir := os.Args[2]
		parentPID, err := strconv.Atoi(os.Args[3])
		if err != nil {
			os.Exit(2)
		}
		os.Exit(connect.RunHelper(workdir, parentPID))
	}

	// Normal TUI path: run in the user's terminal, never elevated.  The
	// elevation prompt (UAC) appears only when connecting, exactly like
	// sudo prompts on Linux.
	favPath, err := favstore.DefaultPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine config path: %v\n", err)
		os.Exit(1)
	}
	store := favstore.New(favPath)
	if err := store.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot load favorites: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(ui.InitialModel(store), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
