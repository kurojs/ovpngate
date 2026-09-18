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

func checkOpenVPN() error {
	if _, err := openvpnBinary(); err != nil {
		return fmt.Errorf("openvpn not found: install from https://openvpn.net/community-downloads/")
	}
	return nil
}

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

	currentHelperPID = 0
	skipCleanup = true
}

func findTunnelIP() (string, bool) {
	if currentLogPath == "" {
		return "", false
	}
	data, err := os.ReadFile(currentLogPath)
	if err != nil {
		return "", false
	}

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

func ensureElevation() error {
	return nil
}
