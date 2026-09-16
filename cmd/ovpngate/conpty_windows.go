//go:build windows

package main

import (
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// conptyMarker is the environment variable that marks a process relaunched
// inside our own pseudo console.  It prevents infinite recursion: the child
// skips the relaunch because isConPTYChild() is true for it.
const conptyMarker = "OVPNGATE_CONPTY=1"

// conptyResizeInterval is how often the resize relay polls the real console
// window size while the child runs inside the pseudo console.
const conptyResizeInterval = 100 * time.Millisecond

// isConPTYChild reports whether this process was relaunched inside a pseudo
// console by runInsideConPTY.  Such processes must never try to relaunch
// again.
func isConPTYChild() bool {
	return os.Getenv("OVPNGATE_CONPTY") == "1"
}

// isLegacyConsole reports whether stdout is attached to a legacy console
// host (cmd.exe / classic PowerShell conhost) instead of a modern terminal
// backed by ConPTY (Windows Terminal, WezTerm, herdr, ...).
//
// Real console output handles succeed at GetConsoleMode while ConPTY pipes
// and redirected handles fail it, so a successful read already implies a
// real console.  Among real consoles, legacy conhost allocates a tall
// scrollback buffer while ConPTY hosts report a buffer exactly as large as
// the visible window, so "buffer rows > window rows" is a reliable legacy
// marker.
func isLegacyConsole() bool {
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		// Pipe/ConPTY backend: ANSI is handled by the terminal host.
		return false
	}
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(handle, &info); err != nil {
		return false
	}
	bufferRows := int(info.Size.Y)
	windowRows := int(info.Window.Bottom-info.Window.Top) + 1
	return bufferRows > windowRows
}

// clampToInt16 bounds a dimension so it fits safely into the int16 fields of
// windows.Coord (the Windows console dimension type is a signed 16-bit
// value).
func clampToInt16(v int) int16 {
	if v < 0 {
		return 0
	}
	if v > 32767 {
		return 32767
	}
	return int16(v)
}

// buildEnvBlock converts a list of "KEY=VALUE" entries into a UTF-16
// environment block for CREATE_UNICODE_ENVIRONMENT: every entry terminated
// by a NUL and the whole block terminated by a final double NUL.
func buildEnvBlock(entries []string) (*uint16, error) {
	var block []uint16
	for _, entry := range entries {
		utf16, err := windows.UTF16FromString(entry)
		if err != nil {
			return nil, err
		}
		block = append(block, utf16...) // UTF16FromString includes the NUL.
	}
	block = append(block, 0) // Final double NUL terminates the block.
	return &block[0], nil
}

// runInsideConPTY relaunches the current executable inside a new Windows
// pseudo console (ConPTY) attached to our real console input/output
// handles.  Inside the pseudo console stdout is a pipe that the ConPTY
// translates into native console output, so ANSI-dependent TUIs render
// exactly as they do in Windows Terminal.  It blocks until the child exits
// and returns its exit code.
func runInsideConPTY() (code int, err error) {
	w, h := consoleSize()

	in, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return 1, err
	}
	out, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return 1, err
	}
	errHandle, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil {
		// Fall back to the output handle when stderr is unavailable so the
		// child still gets a valid error stream inside the pseudo console.
		errHandle = out
	}

	var hpc windows.Handle
	if err := windows.CreatePseudoConsole(
		windows.Coord{X: clampToInt16(w), Y: clampToInt16(h)}, in, out, 0, &hpc,
	); err != nil {
		return 1, err
	}
	defer windows.ClosePseudoConsole(hpc)

	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return 1, err
	}
	defer attrList.Delete()
	if err := attrList.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(&hpc), unsafe.Sizeof(hpc),
	); err != nil {
		return 1, err
	}

	exe, err := os.Executable()
	if err != nil {
		return 1, err
	}
	args := make([]string, 0, len(os.Args))
	args = append(args, exe)
	args = append(args, os.Args[1:]...)
	cmdline, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return 1, err
	}

	envBlock, err := buildEnvBlock(append(windows.Environ(), conptyMarker))
	if err != nil {
		return 1, err
	}

	si := &windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:        uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags:     windows.STARTF_USESTDHANDLES,
			StdInput:  in,
			StdOutput: out,
			StdErr:    errHandle,
		},
		ProcThreadAttributeList: attrList.List(),
	}

	pi := new(windows.ProcessInformation)
	if err := windows.CreateProcess(
		nil, cmdline, nil, nil, false,
		windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT,
		envBlock, nil, &si.StartupInfo, pi,
	); err != nil {
		return 1, err
	}
	if pi.Thread != 0 {
		_ = windows.CloseHandle(pi.Thread)
	}
	if pi.Process != 0 {
		defer windows.CloseHandle(pi.Process)
	}

	// Relay console window resizes into the pseudo console.  The child never
	// receives native resize events, so poll the visible window size and
	// forward changes.  The WaitGroup guarantees the goroutine has fully
	// exited before the deferred ClosePseudoConsole runs.
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lastW, lastH := 0, 0
		for {
			select {
			case <-done:
				return
			default:
			}
			w, h := consoleSize()
			if w > 0 && h > 0 && (w != lastW || h != lastH) {
				lastW, lastH = w, h
				_ = windows.ResizePseudoConsole(
					hpc, windows.Coord{X: clampToInt16(w), Y: clampToInt16(h)},
				)
			}
			select {
			case <-done:
				return
			case <-time.After(conptyResizeInterval):
			}
		}
	}()

	if _, err := windows.WaitForSingleObject(pi.Process, windows.INFINITE); err != nil {
		close(done)
		wg.Wait()
		return 1, err
	}
	close(done)
	wg.Wait()

	var exitCode uint32
	if err := windows.GetExitCodeProcess(pi.Process, &exitCode); err != nil {
		return 1, err
	}
	return int(exitCode), nil
}
