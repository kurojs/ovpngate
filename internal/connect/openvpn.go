package connect

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	tempDir string

	currentFile       string
	currentLogPath    string
	currentPIDPath    string
	currentAuthPath   string
	currentStatusPath string
	currentPID        int
	currentHelperPID  int
	skipCleanup       bool
)

var errPIDNotReady = errors.New("pid not ready yet")

var ErrCancelConnect = errors.New("connection cancelled")
var cancelCh chan struct{}
var currentCmd *exec.Cmd

func Connect(hostname string, ovpnConfig []byte) (int, error) {
	if err := checkOpenVPN(); err != nil {
		return 0, err
	}

	cancelCh = make(chan struct{})

	if err := prepareTempDir(); err != nil {
		return 0, err
	}

	currentFile = filepath.Join(tempDir, "config.ovpn")
	currentLogPath = filepath.Join(tempDir, "openvpn.log")
	currentPIDPath = filepath.Join(tempDir, "openvpn.pid")
	currentAuthPath = filepath.Join(tempDir, "openvpn.auth")
	currentStatusPath = filepath.Join(tempDir, "openvpn.status")
	currentPID = 0

	if err := ensureElevation(); err != nil {
		return 0, err
	}

	sanitized := sanitizeConfig(ovpnConfig)
	if err := os.WriteFile(currentFile, sanitized, 0600); err != nil {
		return 0, fmt.Errorf("failed to write OVPN config: %w", err)
	}

	args := []string{
		"--config", currentFile,
		"--verb", "4",
		"--status", currentStatusPath, "2",
	}
	if cipher := extractCipher(ovpnConfig); cipher != "" {
		args = append(args,
			"--data-ciphers", "DEFAULT:"+cipher,
			"--data-ciphers-fallback", cipher,
		)
	} else {
		args = append(args,
			"--data-ciphers", "DEFAULT:AES-128-CBC",
			"--data-ciphers-fallback", "AES-128-CBC",
		)
	}
	if needsAuth(ovpnConfig) {
		if err := os.WriteFile(currentAuthPath, []byte("vpn\nvpn\n"), 0600); err != nil {
			return 0, fmt.Errorf("failed to write auth file: %w", err)
		}
		args = append(args, "--auth-user-pass", currentAuthPath)
	}

	logFile, err := os.OpenFile(currentLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	if err := startOpenVPN(args, logFile); err != nil {

		cleanupFiles()
		return 0, err
	}

	_ = os.WriteFile(currentPIDPath, []byte(strconv.Itoa(currentPID)), 0644)
	return currentPID, nil
}

func Cancel() {
	if cancelCh != nil {
		close(cancelCh)
	}
	killCurrentProcess()
	cleanupFiles()
}

func WaitForTunnel() (string, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.After(30 * time.Second)

	for {
		select {
		case <-cancelCh:
			return "", ErrCancelConnect

		case <-deadline:
			if err := logError(); err != nil {
				return "", err
			}
			if line := lastLogLine(); line != "" {
				return "", fmt.Errorf("timeout: tunnel did not come up within 30s (last log: %s)", line)
			}
			return "", fmt.Errorf("timeout: tunnel did not come up within 30s")

		case <-ticker.C:
			if err := logError(); err != nil {
				return "", err
			}
			if ip, ok := findTunnelIP(); ok {
				return ip, nil
			}
		}
	}
}

func Disconnect() error {
	killCurrentProcess()
	cleanupFiles()
	return nil
}

func prepareTempDir() error {
	var err error
	tempDir, err = os.MkdirTemp("", "ovpngate-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	return nil
}

func cleanupFiles() {
	if tempDir != "" && !skipCleanup {
		_ = os.RemoveAll(tempDir)
		tempDir = ""
	}
	currentFile = ""
	currentLogPath = ""
	currentPIDPath = ""
	currentAuthPath = ""
	currentStatusPath = ""
	currentPID = 0
	currentCmd = nil
	skipCleanup = false
}

func needsAuth(ovpnConfig []byte) bool {
	return strings.Contains(string(ovpnConfig), "auth-user-pass")
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "vpn"
	}
	return b.String()
}

func logError() error {
	lines := readLogLines()
	if len(lines) == 0 {
		return nil
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "failed to negotiate cipher"):
			return fmt.Errorf("cipher negotiation failed: allow AES-128-CBC")
		case strings.Contains(lower, "auth-failure"),
			strings.Contains(lower, "auth failure"),
			strings.Contains(lower, "auth_failed"),
			strings.Contains(lower, "authentication failed"):
			return fmt.Errorf("auth failed: use username/password vpn/vpn")
		case strings.Contains(lower, "tls error"):
			return fmt.Errorf("tls error: %s", line)
		case strings.Contains(lower, "cannot resolve"):
			return fmt.Errorf("dns error: %s", line)
		case strings.Contains(lower, "connection reset"):
			return fmt.Errorf("connection reset: %s", line)
		case strings.Contains(lower, "fatal") || strings.Contains(lower, "exiting"):
			return fmt.Errorf("openvpn: %s", line)
		}
	}
	return nil
}

func lastLogLine() string {
	lines := readLogLines()
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		return line
	}
	return ""
}

func readLogLines() []string {
	if currentLogPath == "" {
		return nil
	}
	data, err := os.ReadFile(currentLogPath)
	if err != nil {
		return nil
	}
	clean := strings.ReplaceAll(string(data), "\x00", "\n")
	return strings.Split(clean, "\n")
}

func extractCipher(ovpnConfig []byte) string {
	lines := strings.Split(string(ovpnConfig), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[0], "cipher") {
			return fields[1]
		}
	}
	return ""
}

func sanitizeConfig(raw []byte) []byte {
	lines := strings.Split(string(raw), "\n")
	out := make([]string, 0, len(lines))
	skipKeys := map[string]struct{}{
		"log":            {},
		"log-append":     {},
		"status":         {},
		"daemon":         {},
		"writepid":       {},
		"verb":           {},
		"persist-key":    {},
		"syslog":         {},
		"management":     {},
		"auth-user-pass": {},
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			out = append(out, line)
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		if _, skip := skipKeys[strings.ToLower(fields[0])]; skip {
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}
