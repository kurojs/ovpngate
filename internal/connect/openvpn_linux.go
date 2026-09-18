//go:build linux

package connect

import (
	"os/exec"
	"strings"
)

func findTunnelIP() (string, bool) {
	out, err := exec.Command("ip", "-o", "-4", "addr", "show").Output()
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		iface := fields[1]
		if strings.HasPrefix(iface, "tun") || strings.HasPrefix(iface, "tap") {
			ip := strings.Split(fields[3], "/")[0]
			return ip, true
		}
	}
	return "", false
}
