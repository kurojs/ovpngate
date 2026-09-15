//go:build windows

package connect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// checkOpenVPN verifies openvpn is available on the system PATH, falling
// back to the standard install location.
func checkOpenVPN() error {
	if _, err := openvpnBinary(); err != nil {
		return fmt.Errorf("openvpn not found: install from https://openvpn.net/community-downloads/")
	}
	return nil
}

// openvpnBinary resolves the full path to the OpenVPN executable: first via
// PATH lookup, then via the standard Program Files install location.
func openvpnBinary() (string, error) {
	if p, err := exec.LookPath("openvpn"); err == nil {
		return p, nil
	}
	defaultPath := filepath.Join(os.Getenv("ProgramFiles"), "OpenVPN", "bin", "openvpn.exe")
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath, nil
	}
	return "", fmt.Errorf("openvpn executable not found")
}

// startOpenVPN launches OpenVPN through the elevated helper instance.  The
// TUI itself never runs elevated: it writes the helper spec, asks Windows to
// start a helper copy of this executable with "runas" (UAC), and waits for
// the helper to report OpenVPN's PID.  Killing works the same way — the
// helper is the only process that can terminate its elevated OpenVPN child.
func startOpenVPN(args []string, logFile *os.File) error {
	bin, err := openvpnBinary()
	if err != nil {
		return err
	}

	spec := helperSpec{Exe: bin, Args: args}
	specData, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to encode helper spec: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "helper.json"), specData, 0600); err != nil {
		return fmt.Errorf("failed to write helper spec: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("current executable missing: %w", err)
	}

	// PowerShell quoting is safe here: single quotes are invalid in Windows
	// paths, so embedding the exe and workdir in single quotes never breaks.
	workdir := filepath.Clean(tempDir)
	parentPID := os.Getpid()
	psCmd := fmt.Sprintf(
		"Start-Process -FilePath '%s' -ArgumentList '--helper','%s','%d' -Verb RunAs -WindowStyle Hidden",
		exe, workdir, parentPID,
	)
	launcher := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", psCmd)
	launcher.Stdout = logFile
	var errBuf bytes.Buffer
	launcher.Stderr = &errBuf

	if err := launcher.Run(); err != nil {
		return fmt.Errorf("elevation denied or failed: %v (%s)", err, strings.TrimSpace(errBuf.String()))
	}

	// The helper writes helper.pid and openvpn.pid shortly after launch.
	if err := waitForPIDFile(openvpnPIDPath(), 5*time.Second); err != nil {
		return fmt.Errorf("openvpn did not start after elevation: %w", err)
	}
	pid, err := readPID(openvpnPIDPath())
	if err != nil {
		return fmt.Errorf("openvpn pid unreadable: %w", err)
	}
	currentPID = pid
	currentHelperPID, _ = readPID(helperPIDPath())
	return nil
}

func openvpnPIDPath() string { return filepath.Join(tempDir, "openvpn.pid") }
func helperPIDPath() string  { return filepath.Join(tempDir, "helper.pid") }

// waitForPIDFile polls until the pid file exists (or the deadline passes).
func waitForPIDFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("pid file %s not created within timeout", filepath.Base(path))
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// killCurrentProcess signals the elevated helper via the cancel file and
// waits for it to shut OpenVPN down.  If the helper refuses to exit we keep
// the temp dir so it can still reach the cancel marker; the next disconnect
// attempt finishes the cleanup.
func killCurrentProcess() {
	if tempDir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(tempDir, "cancel"), []byte("1"), 0600)

	if currentHelperPID == 0 {
		skipCleanup = false
		return
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(currentHelperPID) {
			currentHelperPID = 0
			skipCleanup = false
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Helper is stuck: preserve tempDir so a later attempt can finish.
	currentHelperPID = 0
	skipCleanup = true
}

// findTunnelIP detects the tunnel IP by parsing the OpenVPN log.
// OpenVPN on Windows never logs a standalone "ifconfig" line (that is the
// Unix format); the assigned address appears either embedded inside the
// PUSH_REPLY control message or in the TAP-Windows DHCP notification.
// All three shapes are matched so detection is robust across versions.
func findTunnelIP() (string, bool) {
	if currentLogPath == "" {
		return "", false
	}
	data, err := os.ReadFile(currentLogPath)
	if err != nil {
		return "", false
	}

	// 1) Unix-style line: "ifconfig 10.x.x.x 10.x.x.x netmask ..."
	// 2) Windows PUSH_REPLY: "... PUSH_REPLY,ping 3,...,ifconfig 10.x.x.x 10.x.x.x,route-gateway ..."
	// 3) Windows TAP notify: "Notified TAP-Windows driver to set a DHCP IP/netmask of 10.x.x.x/255..."
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ifconfig") {
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				ip := fields[1]
				if strings.Contains(ip, ".") {
					return ip, true
				}
			}
			continue
		}
		if strings.Contains(trimmed, "PUSH_REPLY") {
			if ip, ok := ipFromLine(trimmed); ok {
				return ip, true
			}
		}
		if strings.Contains(trimmed, "Notified TAP-Windows driver to set a DHCP IP/netmask of") {
			if ip, ok := ipFromLine(trimmed); ok {
				return ip, true
			}
		}
	}
	return "", false
}

// ipFromLine extracts the first IPv4 address that appears to the right of an
// "ifconfig" marker (PUSH_REPLY embeds "ifconfig <local> <remote>,route-...")
// or the first dotted quad found in the line as a fallback.
func ipFromLine(line string) (string, bool) {
	lower := strings.ToLower(line)
	for _, marker := range []string{"ifconfig ", "ip/netmask of "} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			rest := line[idx+len(marker):]
			fields := strings.FieldsFunc(rest, func(r rune) bool {
				return r == ' ' || r == ',' || r == '/' || r == '\t'
			})
			if len(fields) >= 1 && strings.Contains(fields[0], ".") {
				return fields[0], true
			}
		}
	}
	return "", false
}

// ensureElevation is a no-op on Windows: the TUI never needs elevation; the
// OpenVPN child gets it through the elevated helper at connect time.
func ensureElevation() error {
	return nil
}
