package main

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kurojs/ovpngate/internal/favstore"
	"github.com/kurojs/ovpngate/internal/ui"
)

func main() {
	if os.Geteuid() != 0 {
		cmd := exec.Command("sudo", "-v")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "error: sudo authentication failed\n")
			os.Exit(1)
		}
	}

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
