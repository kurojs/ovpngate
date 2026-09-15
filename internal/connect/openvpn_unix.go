//go:build unix

package connect

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// checkOpenVPN verifies openvpn is available on the system PATH.
func checkOpenVPN() error {
	if _, err := exec.LookPath("openvpn"); err != nil {
		return fmt.Errorf("openvpn not found: install it first (e.g. 'sudo pacman -S openvpn' or 'brew install openvpn')")
	}
	return nil
}

// killCurrentProcess terminates OpenVPN gracefully with SIGTERM.
func killCurrentProcess() {
	if currentCmd != nil && currentCmd.Process != nil {
		_ = currentCmd.Process.Signal(syscall.SIGTERM)
		_ = currentCmd.Wait()
	}
}

// startOpenVPN launches OpenVPN directly, escalating via sudo when needed,
// and records the running process for later cancellation.
func startOpenVPN(args []string, logFile *os.File) error {
	cmd := openvpnCmd(args)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start OpenVPN: %w", err)
	}
	currentCmd = cmd
	currentPID = cmd.Process.Pid
	return nil
}

// openvpnCmd builds the OpenVPN command, escalating via sudo when needed.
func openvpnCmd(args []string) *exec.Cmd {
	if os.Getuid() == 0 {
		return exec.Command("openvpn", args...)
	}
	full := append([]string{"openvpn"}, args...)
	return exec.Command("sudo", full...)
}

// ensureElevation verifies sudo credentials are cached for the session.
func ensureElevation() error {
	if os.Getuid() == 0 {
		return nil
	}
	if err := exec.Command("sudo", "-n", "true").Run(); err != nil {
		return fmt.Errorf("sudo credentials expired: run 'sudo -v' again")
	}
	return nil
}
