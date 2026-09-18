//go:build windows

package main

// main_windows.go — Windows entry point.
//
// Windows renders with tcell (see tui_windows.go).  bubbletea stays ONLY
// on mac/linux (internal/ui, untouched).
//
// Modes handled here:
//   - default          : TUI (runTUI)
//   - --version        : print version and exit
//   - --console-diagnose : diagnostics and exit
//   - --helper workdir parentPID : elevated companion that runs OpenVPN
//     (connect.RunHelper).  startOpenVPN relaunches THIS executable with
//     "runas" when the user connects.

import (
	"fmt"
	"os"
	"strconv"

	"github.com/kurojs/ovpngate/internal/connect"
)

func main() {
	switch {
	case len(os.Args) == 4 && os.Args[1] == "--helper":
		parentPID, err := strconv.Atoi(os.Args[3])
		if err != nil {
			os.Exit(3)
		}
		os.Exit(connect.RunHelper(os.Args[2], parentPID))

	case len(os.Args) == 2 && os.Args[1] == "--version":
		fmt.Println(Version)
		return

	case len(os.Args) == 2 && os.Args[1] == "--console-diagnose":
		consoleDump()
		return
	}

	os.Exit(runTUI())
}
