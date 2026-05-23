package connect 

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var currentFile string

func Connect(hostname string, ovpnConfig []byte) (int, error) {
	path := fmt.Sprintf("/tmp/opvngate-%s.opvn", hostname)
	currentFile = path

	err := os.WriteFile(path, ovpnConfig, 0600)
	if err != nil {
		return 0, fmt.Errorf("failed to write OVPN config: %w", err)
	}

	cmd :=  exec.Command("sudo", "openvpn", "--config", path, "--daemon",
	"--log", "/tmp/ovpngate.log")
	
	err = cmd.Run()
	if err != nil {	
		return 0, fmt.Errorf("failed to start OpenVPN: %w", err)
	}

	return cmd.Process.Pid, nil

}

func WaitForTunnel() (string, error) {
	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return "", fmt.Errorf("timeout: tunnel no levantó en 15s")
		case <-ticker.C:
			out, err := exec.Command("ip", "addr", "show", "tun0").Output()
			if err != nil {
				continue
			}
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "inet ") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						ip := strings.Split(parts[1], "/")[0]
						return ip, nil
					}
				}
			}
		}
	}
}

func Disconnect() error {
	err := exec.Command("sudo", "pkill", "-f", "ovpngate").Run()
	if currentFile != "" {
		os.Remove(currentFile)
		currentFile = ""
	}
	return err
}
