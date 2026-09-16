//go:build windows

package main

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// captureOutput tees every byte written to the real terminal output into a
// capture file as well, then forwards the payload to its target (the raw or
// ConPTY-safe stream).  It embeds *os.File so it still satisfies term.File
// (the Windows tty detection in bubbletea asserts p.output.(term.File) and
// reads Fd()), while Write duplicates the payload to the capture file.  Used
// only when OVPNGATE_CAPTURE_OUTPUT is set, to record the exact ANSI stream
// the TUI emits.
type captureOutput struct {
	*os.File
	capture *os.File
	target  io.Writer
}

func (c *captureOutput) Write(p []byte) (int, error) {
	_, _ = c.capture.Write(p)
	return c.target.Write(p)
}

// WriteString intercepts the string fast path as well.  The embedded *os.File
// would otherwise promote its own WriteString and bubbletea's renderer uses
// io.WriteString(r.out, seq) for every terminal control sequence (hide cursor,
// alt screen enter/exit, bracketed paste, ...).  Those sequences must reach
// the capture file too, or the capture looks like the TUI never entered the
// alternate screen when it actually did.
func (c *captureOutput) WriteString(s string) (int, error) {
	_, _ = c.capture.WriteString(s)
	if sw, ok := c.target.(io.StringWriter); ok {
		return sw.WriteString(s)
	}
	return c.target.Write([]byte(s))
}

// consoleDump prints a structured report of what the running terminal
// backend actually reports through the Windows console APIs.  It exists to
// stop guessing: the legacy-conhost vs ConPTY distinction, whether virtual
// terminal processing is (or can be) enabled, and the real visible buffer
// size are all facts we need before choosing the rendering mode.
func consoleDump() {
	fmt.Println("== ovpngate console diagnostic ==")
	fmt.Println(Version)

	ver := windows.RtlGetVersion()
	if ver != nil {
		fmt.Printf("Windows: %d.%d build %d\n", ver.MajorVersion, ver.MinorVersion, ver.BuildNumber)
	}

	dumpHandle := func(name string, handle windows.Handle) {
		fmt.Printf("-- %s --\n", name)

		ft, err := windows.GetFileType(handle)
		if err != nil {
			fmt.Printf("GetFileType: error %v\n", err)
		} else {
			t := "unknown"
			switch ft {
			case windows.FILE_TYPE_CHAR:
				t = "CHAR (real console)"
			case windows.FILE_TYPE_DISK:
				t = "DISK (file)"
			case windows.FILE_TYPE_PIPE:
				t = "PIPE (ConPTY host or redirect)"
			case windows.FILE_TYPE_REMOTE:
				t = "REMOTE"
			}
			fmt.Printf("GetFileType: 0x%X (%s)\n", ft, t)
		}

		var mode uint32
		err = windows.GetConsoleMode(handle, &mode)
		if err != nil {
			fmt.Printf("GetConsoleMode: ERROR %v\n", err)
			return
		}
		fmt.Printf("GetConsoleMode: ok, mode=0x%X\n", mode)
		if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
			fmt.Println("  ENABLE_VIRTUAL_TERMINAL_PROCESSING: ON")
		} else {
			fmt.Println("  ENABLE_VIRTUAL_TERMINAL_PROCESSING: OFF")
		}
		if mode&windows.ENABLE_WRAP_AT_EOL_OUTPUT != 0 {
			fmt.Println("  ENABLE_WRAP_AT_EOL_OUTPUT: ON")
		}
		if mode&windows.ENABLE_PROCESSED_OUTPUT != 0 {
			fmt.Println("  ENABLE_PROCESSED_OUTPUT: ON")
		}
	}

	out, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		fmt.Println("GetStdHandle(STDOUT): error", err)
	} else {
		dumpHandle("STDOUT", out)
	}

	out2, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		fmt.Println("GetStdHandle(STDIN): error", err)
	} else {
		dumpHandle("STDIN", out2)
	}

	// Try to enable VT on the output handle and confirm it sticks.
	if err == nil {
		var mode uint32
		if werr := windows.GetConsoleMode(out, &mode); werr == nil {
			before := mode
			_ = windows.SetConsoleMode(out, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
			_ = windows.GetConsoleMode(out, &mode)
			if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
				fmt.Println("SetConsoleMode(VT): ON after enable")
			} else {
				fmt.Println("SetConsoleMode(VT): COULD NOT ENABLE (mode unchanged)")
			}
			_ = windows.SetConsoleMode(out, before)
		}
	}

	if ft, ferr := windows.GetFileType(out); ferr == nil && ft == windows.FILE_TYPE_CHAR {
		var info windows.ConsoleScreenBufferInfo
		if werr := windows.GetConsoleScreenBufferInfo(out, &info); werr == nil {
			fmt.Printf("Buffer size: %dx%d\n", info.Size.X, info.Size.Y)
			fmt.Printf("Window:      left=%d top=%d right=%d bottom=%d\n",
				info.Window.Left, info.Window.Top, info.Window.Right, info.Window.Bottom)
			fmt.Printf("Visible rows: %d (buffer rows: %d)\n",
				info.Window.Bottom-info.Window.Top+1, info.Size.Y)
			if info.Size.Y > info.Window.Bottom-info.Window.Top+1 {
				fmt.Println("  => BUFFER TALLER THAN WINDOW (scrollback present)")
			}
		} else {
			fmt.Println("GetConsoleScreenBufferInfo: error", werr)
		}
	}

	fmt.Println("-- environment --")
	for _, k := range []string{
		"WT_SESSION", "WT_PROFILE_ID", "TERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION",
		"ConEmuANSI", "ANSICON", "MSYSTEM", "OVPNGATE_CONPTY",
	} {
		if v, ok := os.LookupEnv(k); ok {
			fmt.Printf("%s=%s\n", k, v)
		} else {
			fmt.Printf("%s=<unset>\n", k)
		}
	}

	fmt.Println("isConPTYChild:", isConPTYChild())
	fmt.Println("isLegacyConsole:", isLegacyConsole())

	fmt.Println("-- app decisions --")
	vtOK := enableVT()
	fmt.Println("enableVT():", vtOK)
	fmt.Println("useAltScreen():", useAltScreen())
	w, h := consoleSize()
	fmt.Printf("consoleSize(): %dx%d\n", w, h)
	fmt.Println("withAltScreen option applied:", useAltScreen() || vtOK)
	visible := h - 12
	if visible < 6 {
		visible = 6
	}
	fmt.Printf("listVisibleCount (height-12, min 6): %d\n", visible)
	fmt.Println("== end ==")
}