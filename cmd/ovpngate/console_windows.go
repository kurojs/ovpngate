//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procSetConsoleScreenBuffer = modkernel32.NewProc("SetConsoleScreenBufferSize")
)

func setConsoleScreenBufferSize(handle windows.Handle, size windows.Coord) error {
	r1, _, e1 := syscall.Syscall(procSetConsoleScreenBuffer.Addr(), 2,
		uintptr(handle), uintptr(*(*uint32)(unsafe.Pointer(&size))), 0)
	if r1 == 0 {
		return e1
	}
	return nil
}

// enableVT turns on Virtual Terminal Processing on the console output
// handle.  Without it a legacy conhost ignores the ANSI sequences Bubble
// Tea relies on: the alternate screen is never entered (no full screen) and
// repaint-by-cursor (CursorHome/CursorUp) does nothing, so every frame is
// appended below the previous one like shell output.  Bubble Tea tries to
// enable this itself, but in some setups the mode does not stick, so we set
// it explicitly before the program starts.  Returns true when VT processing
// is confirmed active.
func enableVT() bool {
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// Not a real console (ConPTY pipe): the host handles ANSI itself.
		return true
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING == 0 {
		if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
			return false
		}
	}
	return true
}

// useAltScreen reports whether the output is backed by a modern terminal
// (ConPTY: Windows Terminal, WezTerm, herdr, etc.) where the alternate
// screen buffer and ANSI repaint work reliably.  Children relaunched inside
// our own pseudo console by runInsideConPTY are modern backends too: their
// stdout is a ConPTY pipe translated to native console output, so the alt
// screen works there exactly as in any ConPTY terminal.  On the other
// backends stdout is a pipe and GetConsoleMode fails (modern), while on a
// real legacy console (cmd.exe / PowerShell classic conhost) GetConsoleMode
// succeeds, and the alt screen is unreliable: frames end up stacked instead
// of repainted in place, so Bubble Tea runs in inline mode there instead.
func useAltScreen() bool {
	if isConPTYChild() {
		return true
	}
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return true
	}
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// Not a real console: modern ConPTY/pipe backend.
		return true
	}
	// Real legacy console: alt screen repaint is unreliable.
	return false
}

// fitConsoleBufferToWindow shrinks the console buffer down to the visible
// window.  Legacy conhost (cmd.exe / Windows PowerShell classic) clones the
// primary buffer size into the alternate screen buffer: if the primary
// buffer has thousands of scrollback rows, the alt screen does too, and the
// Bubble Tea TUI renders as an unbounded scrolling wall instead of a fixed
// full-screen view.  Modern terminals back by ConPTY (Windows Terminal,
// WezTerm, etc.) already report a window-sized buffer, so this is a no-op
// there.
func fitConsoleBufferToWindow() {
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(handle, &info); err != nil {
		return
	}
	cols := info.Window.Right - info.Window.Left + 1
	rows := info.Window.Bottom - info.Window.Top + 1
	if cols <= 0 || rows <= 0 {
		return
	}
	if info.Size.X > cols || info.Size.Y > rows {
		_ = setConsoleScreenBufferSize(handle, windows.Coord{X: cols, Y: rows})
	}
}

// consoleSize returns the size of the console's visible window, not its
// scrollback buffer.  Legacy Windows consoles (cmd.exe / Windows PowerShell
// on the classic conhost) report the buffer size as the terminal size — a
// buffer can be thousands of rows tall, which makes the TUI render as if it
// had no fixed bounds and scroll like a regular shell output.  Reading the
// window bounds gives the real visible area the user sees.
func consoleSize() (width, height int) {
	width, height = 80, 24
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return width, height
	}
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(handle, &info); err != nil {
		return width, height
	}
	cols := int(info.Window.Right-info.Window.Left) + 1
	rows := int(info.Window.Bottom-info.Window.Top) + 1
	if cols > 0 {
		width = cols
	}
	if rows > 0 {
		height = rows
	}
	return width, height
}
