//go:build windows

package main

import (
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const conptyMarker = "OVPNGATE_CONPTY=1"

const conptyResizeInterval = 100 * time.Millisecond

func isConPTYChild() bool {
	return os.Getenv("OVPNGATE_CONPTY") == "1"
}

func isLegacyConsole() bool {
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {

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

func clampToInt16(v int) int16 {
	if v < 0 {
		return 0
	}
	if v > 32767 {
		return 32767
	}
	return int16(v)
}

func buildEnvBlock(entries []string) (*uint16, error) {
	var block []uint16
	for _, entry := range entries {
		utf16, err := windows.UTF16FromString(entry)
		if err != nil {
			return nil, err
		}
		block = append(block, utf16...)
	}
	block = append(block, 0)
	return &block[0], nil
}

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
