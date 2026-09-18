//go:build darwin

package connect

import (
	"os/exec"
	"strings"
)

func findTunnelIP() (string, bool) {
	out, err := exec.Command("ifconfig").Output()
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(out), "\n")
	var currentIface string
	for _, line := range lines {

		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
			currentIface = strings.SplitN(line, ":", 2)[0]
			continue
		}
		if strings.HasPrefix(currentIface, "tun") || strings.HasPrefix(currentIface, "tap") || strings.HasPrefix(currentIface, "utun") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "inet ") {
				fields := strings.Fields(trimmed)
				if len(fields) >= 2 {
					return fields[1], true
				}
			}
		}
	}
	return "", false
}
