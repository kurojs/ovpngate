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

func enableVT() bool {
	handle, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {

		return true
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING == 0 {
		if err := windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
			return false
		}
	}
	return true
}

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

		return true
	}

	return false
}

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
