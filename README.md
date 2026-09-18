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
- Automatic privilege elevation at CONNECT time only (sudo on Linux/macOS, UAC on Windows) -- the TUI itself always runs unprivileged in your own terminal
- Sanitized OpenVPN configuration (strips unsafe directives)
- Cipher auto-detection from server config
- Temporary per-run directory with full cleanup
- Connection timeout with diagnostic log capture

## Prerequisites

- [OpenVPN](https://openvpn.net/) -- the underlying VPN client
- Admin privileges -- for OpenVPN TUN/TAP device creation

Platform-specific:

| Platform | OpenVPN install | Tunnel IP detection | Elevation |
|----------|-----------------|-------------------|-----------|
| Linux    | `sudo pacman -S openvpn` or distro equivalent | `iproute2` (`ip addr show`) | `sudo` |
| macOS    | `brew install openvpn` | `ifconfig` (built-in) | `sudo` |
| Windows  | [OpenVPN Community Installer](https://openvpn.net/community-downloads/) | OpenVPN log parsing (`findTunnelIP`) | UAC prompt at connect |

> [!NOTE]
> Installing via AUR (`yay -S ovpngate`) pulls all dependencies automatically on Arch Linux.

## Installation

### Windows (recommended)

Run the installer in PowerShell. It downloads the latest release binary, installs it to `%LOCALAPPDATA%\ovpngate\ovpngate.exe`, and adds that directory to your user PATH:

```powershell
irm https://raw.githubusercontent.com/kurojs/ovpngate/main/install.ps1 | iex
```

Re-running it updates to the latest release only when the installed version differs. Verify with:

```powershell
ovpngate --version
```

### Linux (AUR, recommended for Arch)

```bash
yay -S ovpngate
```

### Prebuilt binaries (Linux / macOS / Windows)

Download the archive for your platform from the [latest release](https://github.com/kurojs/ovpngate/releases/latest) and verify it against `checksums.txt`:

| Asset | Platform |
|-------|----------|
| `ovpngate-windows-amd64.exe` | Windows x86_64 |
| `ovpngate-linux-amd64` / `ovpngate-linux-arm64` | Linux |
| `ovpngate-darwin-amd64` / `ovpngate-darwin-arm64` | macOS |

### With Go installed

```bash
go install github.com/kurojs/ovpngate/cmd/ovpngate@latest
```

## Usage

Run the program:

```bash
ovpngate
```

On first launch the server list loads automatically -- no elevation is requested yet. Elevation happens only when you actually connect: `sudo` (Linux/macOS) or a UAC admin prompt (Windows). The TUI itself always runs unprivileged in your own terminal.

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

1. The program stages the connection: on Windows an elevated helper is spawned via UAC (PowerShell `Start-Process -Verb RunAs`), which alone has the privileges to launch OpenVPN; on Unix `sudo` is used directly.
2. A temporary directory is created for all runtime artifacts.
3. The raw OpenVPN config is sanitized: unsafe directives (`daemon`, `log`, `writepid`, `persist-key`, `auth-user-pass`, etc.) are removed.
4. The sanitized config and an auth file with public credentials (`vpn`/`vpn`) are written to the temp directory.
5. The required cipher is extracted from the server config and set explicitly.
6. OpenVPN is launched with the prepared config, redirecting output to a log file.
7. The TUI polls every 500ms for a `tun`/`tap` interface (via `ip addr show` on Linux, `ifconfig` on macOS, or OpenVPN log parsing on Windows).
8. When the tunnel interface appears, the assigned IP is captured and the connection is considered active.
9. OpenVPN log output is monitored for known error patterns (cipher mismatch, auth failure, TLS errors, DNS failure, connection reset) and surfaced immediately.

On disconnect, the OpenVPN process is terminated and the temporary directory is removed.

## Project structure

```
cmd/ovpngate/
  main_unix.go         Entry point, sudo prompt, Bubble Tea bootstrap (Linux/macOS/BSD)
  main_windows.go      Entry point, helper dispatch, Bubble Tea bootstrap (Windows)

internal/connect/
  openvpn.go           Shared OpenVPN lifecycle: Connect, WaitForTunnel, Cancel, Disconnect
  openvpn_unix.go      Unix helpers: sudo elevation, startOpenVPN hook, SIGTERM lifecycle
  openvpn_linux.go     Linux tunnel IP detection (iproute2)
  openvpn_darwin.go    macOS tunnel IP detection (ifconfig)
  openvpn_windows.go   Windows helpers: PowerShell RunAs elevation, helper protocol, log parsing (findTunnelIP)
  helper_windows.go    Elevated OpenVPN/helper mode (RunHelper, helperSpec, cancel file protocol)

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
Install OpenVPN: `sudo pacman -S openvpn` (Arch), `brew install openvpn` (macOS), or the [OpenVPN Community Installer](https://openvpn.net/community-downloads/) (Windows).

**"sudo authentication failed"** / **UAC prompt denied**  
Privileges are required to run OpenVPN for TUN/TAP device creation. Enter your sudo password or accept the UAC prompt. On Windows you can also right-click the executable and select "Run as administrator".

**Connection timeout**  
Public VPN relays can be slow or saturated. Try a different server -- those with lower session counts and higher speeds are more reliable. The last log line is included in the error message to help diagnose the issue.

**Cipher negotiation failed**  
The server cipher is detected and set in the OpenVPN arguments, but some servers may advertise unsupported ciphers. Try a different server.

## License

MIT
