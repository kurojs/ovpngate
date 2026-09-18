//go:build windows

package connect

import "testing"

func TestFindTunnelIPWindowsShapes(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "standalone ifconfig (unix compat)",
			line: "2026-09-15 01:05:57 us=609000 ifconfig 10.239.200.97 10.239.200.98 netmask 255.255.255.252",
			want: "10.239.200.97",
		},
		{
			name: "PUSH_REPLY embedded ifconfig",
			line: "2026-09-15 01:05:52 us=109000 PUSH: Received control message: 'PUSH_REPLY,ping 3,ping-restart 10,ifconfig 10.239.200.97 10.239.200.98,dhcp-option DNS 10.239.254.254,dhcp-option DNS 8.8.8.8,route-gateway 10.239.200.98,redirect-gateway def1'",
			want: "10.239.200.97",
		},
		{
			name: "TAP-Windows DHCP notify",
			line: "2026-09-15 01:05:52 us=140000 Notified TAP-Windows driver to set a DHCP IP/netmask of 10.239.200.97/255.255.255.252 on interface {11E29A6B-84FE-4109-BF8A-6D04049B5236} [DHCP-serv: 10.239.200.98, lease-time: 31536000]",
			want: "10.239.200.97",
		},
		{
			name: "OpenVPN 2.6 non-DCO PUSH_REPLY",
			line: "2024-05-01 12:00:00 us=000000 PUSH: Received control message: 'PUSH_REPLY,ifconfig 10.76.1.2 10.76.1.1,route-gateway 10.76.1.1,topology subnet,ping 8,ping-restart 30'",
			want: "10.76.1.2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ipFromLine(tc.line)
			if !ok {
				t.Fatalf("no IP extracted from %q", tc.line)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q (line: %q)", tc.want, got, tc.line)
			}
		})
	}
}
