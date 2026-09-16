//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kurojs/ovpngate/internal/connect"
	"github.com/kurojs/ovpngate/internal/favstore"
	"github.com/kurojs/ovpngate/internal/ui"
)

func main() {
	// Version query used by the Windows installer/updater.
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(Version)
		return
	}

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

	// Print a structured report of the current terminal backend and exit.
	// Read-only: runs in the real terminal before any ConPTY relaunch.
	if len(os.Args) == 2 && os.Args[1] == "--console-diagnose" {
		consoleDump()
		return
	}

	// ConPTY self-relaunch is DISABLED by default.  The console diagnostic
	// proved that the modern conhost (Windows 10 build 28120 and later) can
	// enable ENABLE_VIRTUAL_TERMINAL_PROCESSING directly on a legacy
	// console, so no relaunch is needed for the TUI to work in cmd.exe or
	// classic PowerShell.  Worse, the previous auto-relaunch used
	// STARTF_USESTDHANDLES with the real console handles, contradicting
	// Microsoft's documented ConPTY pattern (pipes +
	// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, no STARTF_USESTDHANDLES), and it
	// made the app fail to open in those terminals.  Re-enable only with
	// OVPNGATE_CONPTY_WRAP=1 once the wrapper follows the official sample.
	if os.Getenv("OVPNGATE_CONPTY_WRAP") == "1" && !isConPTYChild() && isLegacyConsole() {
		code, err := runInsideConPTY()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: cannot start pseudo console:", err)
			os.Exit(1)
		}
		os.Exit(code)
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

	// Make sure the console processes ANSI escapes (alternate screen, cursor
	// movement, clear).  Without VT processing every Bubble Tea frame gets
	// appended below the previous one and the TUI never becomes full screen.
	// ConPTY backends already handle ANSI; legacy consoles get the flag set
	// explicitly here.
	vtOK := enableVT()

	// Choose the rendering mode based on the terminal backend.  Modern
	// ConPTY terminals (Windows Terminal, WezTerm, herdr, ...) get the
	// alternate screen; legacy console hosts get inline repaint, because
	// their alt screen stacks frames instead of repainting in place.
	fitConsoleBufferToWindow()

	// Legacy Windows consoles report the scrollback buffer size instead of
	// the visible window on WindowSizeMsg, which makes the TUI render as if
	// it had no fixed bounds.  Bubbletea does not receive resize events on
	// Windows either, so correct every size message with the real visible
	// console size.
	opts := []tea.ProgramOption{
		tea.WithFilter(func(m tea.Model, msg tea.Msg) tea.Msg {
			if wm, ok := msg.(tea.WindowSizeMsg); ok {
				w, h := consoleSize()
				if w > 0 && h > 0 {
					wm.Width, wm.Height = w, h
				}
			}
			return msg
		}),
	}
	if useAltScreen() || vtOK {
		opts = append(opts, tea.WithAltScreen())
	}

	// Build the terminal output stream.  The ConPTY-safe wrapper rewrites the
	// frame flush (home + newline-separated rows) into per-row absolute
	// cursor positioning (CUP + erase-to-EOL), the painting pattern
	// tcell/lazygit use.  ConPTY hosts double-advance the cursor on
	// newline-flushed frames, so every row renders twice; absolute per-row
	// positioning renders identically in every backend, including herdr.
	// OVPNGATE_RAW_OUTPUT=1 bypasses the wrapper for byte-level diagnosis.
	var out io.Writer = &conptySafeOutput{File: os.Stdout}
	if os.Getenv("OVPNGATE_RAW_OUTPUT") == "1" {
		out = os.Stdout
	}

	// Debug capture: when OVPNGATE_CAPTURE_OUTPUT points at a file, every
	// byte bubbletea writes to the renderer is also appended there, BEFORE
	// the ConPTY-safe rewrite, so the file holds the raw frame stream.  The
	// wrapper embeds *os.File so bubbletea's Windows tty detection
	// (p.output.(term.File) + Fd()) still sees a real console and enables
	// VT/resize exactly as before; only Write is intercepted.  Off by
	// default.
	if capPath := os.Getenv("OVPNGATE_CAPTURE_OUTPUT"); capPath != "" {
		capFile, err := os.OpenFile(capPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			defer capFile.Close()
			opts = append(opts, tea.WithOutput(&captureOutput{File: os.Stdout, capture: capFile, target: out}))
			fmt.Fprintln(capFile, "== capture start: OVPNGATE_CAPTURE_OUTPUT ==")
		}
	} else {
		// Normal path: bubbletea MUST write through the ConPTY-safe wrapper,
		// not raw os.Stdout.  Without this the renderer's default newline-
		// flushed frame (home + newline-separated rows) goes straight to
		// ConPTY, which double-advances the cursor on every row.  The wrapper
		// rewrites it into per-row absolute CUP + erase-to-EOL, the exact
		// painting pattern tcell/lazygit use and every ConPTY host renders
		// without duplication.
		opts = append(opts, tea.WithOutput(out))
	}

	p := tea.NewProgram(ui.InitialModel(store), opts...)
	if _, err := p.Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
}
