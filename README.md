# ovpngate

A terminal-based OpenVPN client for the [VPN Gate](https://www.vpngate.net/) public relay service powered by [SoftEther VPN](https://github.com/SoftEtherVPN/SoftEtherVPN). Browse the server list, inspect details, and connect to any relay with one keystroke -- all from within a Bubble Tea TUI.

### Server List
<div align="center">
  <table>
    <tr>
      <td><img src="https://i.imgur.com/qLqg45F.png" width="400" alt="Server List"/></td>
      <td><img src="https://i.imgur.com/1Rqkcto.png" width="400" alt="Server Details"/></td>
    </tr>
    <tr>
      <td align="center"><em>Live server list with country flag, ping, speed, sessions, and operator</em></td>
      <td align="center"><em>Server detail with stats, operator message, and connection controls</em></td>
    </tr>
  </table>
</div>

## Features

- Fetches live server list from the VPN Gate API
- Server detail view with ping, speed, session count, operator info, and server message
- Filter servers by all, fastest, favorites, or specific country
- Favorite servers (★) with persistent storage across sessions
- Offline favorite detection — grayed out when no longer available
- Cancel in-flight connections with a single keystroke
- Keyboard-driven navigation with scrollbar and paging
- Automatic sudo elevation at startup (no prompts during use)
- Sanitized OpenVPN configuration (strips unsafe directives)
- Cipher auto-detection from server config
- Temporary per-run directory with full cleanup
- Connection timeout with diagnostic log capture

## Prerequisites

- [OpenVPN](https://openvpn.net/) -- the underlying VPN client
- `sudo` -- for OpenVPN TUN/TAP device creation (preinstalled on most systems)
- `iproute2` -- for tunnel IP detection (preinstalled on most systems)

> [!NOTE]
> Installing via AUR (`yay -S ovpngate`) pulls all dependencies automatically.

## Installation

### AUR (recommended)

```bash
yay -S ovpngate
```

### From source

```bash
git clone https://github.com/kurojs/ovpngate.git
cd ovpngate
go build -ldflags="-s -w" -o ovpngate ./cmd/ovpngate/
sudo cp ovpngate /usr/local/bin/
```

### With Go installed

```bash
go install github.com/kurojs/ovpngate/cmd/ovpngate@latest
```

## Usage

Run the program:

```bash
ovpngate
```

On first launch you will be prompted for your sudo password. This caches credentials for the session so OpenVPN can be launched without further prompts. After that the TUI opens and the server list loads automatically.

### Key bindings

| Key | Context | Action |
|-----|---------|--------|
| `up` / `k` | List | Previous server |
| `down` / `j` | List | Next server |
| `pgup` / `pgdn` | List | Page through servers |
| `enter` | List / Detail | View details / Connect |
| `esc` | Detail / Connecting | Go back / Cancel connection |
| `r` | List | Refresh server list |
| `a` | List | Show all servers |
| `f` | List | Sort by fastest |
| `v` | List | Show only favorites |
| `c` | List | Cycle country filter |
| `s` | List / Detail | Toggle favorite (star) |
| `d` | Connected | Disconnect |
| `q` | Anywhere | Quit |

### Connection workflow

1. Select a server from the list and press `enter` to view details.
2. Press `enter` again to connect. The TUI shows a spinner while OpenVPN starts.
3. Press `esc` at any time during connection to cancel.
4. Once connected, the assigned IP is displayed. Press `d` to disconnect.
5. Press `q` or `Ctrl+C` to exit.

## How it works

ovpngate fetches a CSV list of public OpenVPN relays from the VPN Gate API. Each entry includes server metadata (hostname, IP, ping, bandwidth, session count, country, operator) and a base64-encoded OpenVPN configuration.

When you initiate a connection:

1. The program requests sudo credentials (at launch) to cache them for the session.
2. A temporary directory is created for all runtime artifacts.
3. The raw OpenVPN config is sanitized: unsafe directives (`daemon`, `log`, `writepid`, `persist-key`, `auth-user-pass`, etc.) are removed.
4. The sanitized config and an auth file with public credentials (`vpn`/`vpn`) are written to the temp directory.
5. The required cipher is extracted from the server config and set explicitly.
6. OpenVPN is launched via `sudo` with the prepared config, redirecting output to a log file.
7. The TUI polls `ip addr show` every 500ms for a `tun` or `tap` interface.
8. When the tunnel interface appears, the assigned IP is captured and the connection is considered active.
9. OpenVPN log output is monitored for known error patterns (cipher mismatch, auth failure, TLS errors, DNS failure, connection reset) and surfaced immediately.

On disconnect, the OpenVPN process is terminated and the temporary directory is removed.

## Project structure

```
cmd/ovpngate/
  main.go              Entry point, sudo prompt, Bubble Tea bootstrap

internal/connect/
  openvpn.go           OpenVPN lifecycle: Connect, WaitForTunnel, Cancel, Disconnect

internal/ui/
  model.go             Bubble Tea model, message types, update loop
  list.go              Server list rendering with fixed-width columns
  detail.go            Server detail and connection status rendering
  styles.go            Lipgloss styles and panel primitives

internal/favstore/
  favstore.go          JSON-persisted favorite servers with CRUD operations

internal/vpngate/
  server.go            Server data structure
  fetch.go             HTTP client with 15s timeout and CSV parser
```

## Building from source

```bash
git clone https://github.com/kurojs/ovpngate.git
cd ovpngate
go mod tidy
go build -ldflags="-s -w" -o ovpngate ./cmd/ovpngate/
```

The `-s -w` flags strip debug information, reducing the binary size.

## Troubleshooting

**"openvpn not found"**  
Install OpenVPN: `sudo pacman -S openvpn` (Arch) or your distribution's equivalent.

**"sudo authentication failed"**  
Enter your sudo password when prompted. This is required to run OpenVPN with the privileges needed for TUN/TAP device creation.

**Connection timeout**  
Public VPN relays can be slow or saturated. Try a different server -- those with lower session counts and higher speeds are more reliable. The last log line is included in the error message to help diagnose the issue.

**Cipher negotiation failed**  
The server cipher is detected and set in the OpenVPN arguments, but some servers may advertise unsupported ciphers. Try a different server.

## License

MIT
