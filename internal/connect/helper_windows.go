//go:build windows

package connect

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

// helperSpec is the launch contract written by the non-elevated TUI process
// and consumed by the elevated helper instance.  No secrets live here: the
// auth file is referenced by path, never inlined.
type helperSpec struct {
	Exe  string   `json:"exe"`  // resolved openvpn binary
	Args []string `json:"args"` // full openvpn arguments
}

// RunHelper is the elevated companion mode of ovpngate.  It is launched via
// "runas" exactly when the user connects, runs OpenVPN with the privilege
// level the TUN driver requires, and shuts OpenVPN down when the user
// disconnects (cancel file) or when the owning TUI process dies.
//
// Control is purely file-based to avoid IPC infrastructure:
//   - helper.json  : the spec above (workdir/helper.json)
//   - helper.pid   : this process' PID, written once OpenVPN is started
//   - openvpn.pid  : OpenVPN's PID, written once started
//   - cancel       : when present, kill OpenVPN and exit
//   - parent death : when the TUI process dies, kill OpenVPN and exit
func RunHelper(workdir string, parentPID int) int {
	data, err := os.ReadFile(filepath.Join(workdir, "helper.json"))
	if err != nil {
		return 2
	}
	var spec helperSpec
	if err := json.Unmarshal(data, &spec); err != nil || spec.Exe == "" || len(spec.Args) == 0 {
		return 2
	}

	// Capture OpenVPN's console output into workdir/openvpn.log so the
	// non-elevated TUI can detect the tunnel IP from it.  With nil Stdout/
	// Stderr Go connects the child to NUL on Windowsclo, so the PUSH_REPLY
	// "ifconfig 10.x" / TAP DHCP lines never reach the log and findTunnelIP
	// times out even though the tunnel is actually up — that was the bug.
	logPath := filepath.Join(workdir, "openvpn.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		_ = os.WriteFile(filepath.Join(workdir, "helper.error"), []byte(fmt.Sprintf("log: %v", err)), 0600)
		return 3
	}
	defer logFile.Close()

	cmd := exec.Command(spec.Exe, spec.Args...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = os.WriteFile(filepath.Join(workdir, "helper.error"), []byte(fmt.Sprintf("start: %v", err)), 0600)
		return 3
	}

	// Announce both PIDs, then wait for cancellation.
	_ = os.WriteFile(filepath.Join(workdir, "helper.pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0600)
	_ = os.WriteFile(filepath.Join(workdir, "openvpn.pid"), []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0600)

	exited := make(chan int, 1)
	go func() {
		_ = cmd.Wait()
		exited <- cmd.ProcessState.ExitCode()
	}()

	cancelPath := filepath.Join(workdir, "cancel")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case code := <-exited:
			// OpenVPN exited on its own (server disconnect, fatal error).
			return code

		case <-ticker.C:
			if _, err := os.Stat(cancelPath); err == nil {
				_ = cmd.Process.Kill()
				<-exited
				return 0
			}
			if !processAlive(parentPID) {
				// The TUI is gone — never orphan an elevated VPN.
				_ = cmd.Process.Kill()
				<-exited
				return 1
			}
		}
	}
}

// processAlive reports whether a process with the given PID is still running.
func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == 259 // STATUS_PENDING: still running
}
