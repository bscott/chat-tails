# Chat Tails

A terminal-based chat application built in Go. Share a chat room with friends over your Tailscale network - they connect with the built-in client, netcat, or telnet.

## Features

- **Tailscale Integration** - Share your chat server securely with anyone on your Tailnet
- **Built-in Client** - `chat-tails --client --host <server>` connects with proper terminal handling
- **Zero Client Setup** - Users can still connect with just `nc` or `telnet`
- **Colorful UI** - Each user gets a unique color, styled messages with ANSI colors
- **Message History** - New users can see recent chat history (optional)
- **Chat Commands** - `/who`, `/me`, `/help`, `/quit`
- **Rate Limiting** - Built-in protection against spam

## Quick Start

```bash
# Build
make build

# Run locally
./chat-tails

# Run with Tailscale (share with your network)
export TS_AUTHKEY=tskey-auth-xxxxx
./chat-tails --tailscale --hostname mychat --history
```

After startup, copy one of the connection commands printed by Chat Tails.
Tailscale assigns the authoritative MagicDNS name, which may differ from the
requested node name.

## Connecting

The same binary doubles as the client — no `nc` or `telnet` needed:

```bash
# Connect to a server (port defaults to 2323)
chat-tails --client --host chat.example.ts.net

# Explicit port
chat-tails --client --host 100.101.102.103 --port 2323
```

The client puts your terminal into raw mode for the duration of the session
and always restores it on exit, disconnect, or signal. It connects through
the regular network stack, so Tailscale MagicDNS names and tailnet IPs work
as long as Tailscale is running on your machine. Quit from inside the chat
with `/quit`, `Esc`, or `Ctrl+C`.

Notes:

- The client targets the default (TUI) server mode. For servers started with
  `--plain-text`, keep using `nc` or `telnet`.
- The chat UI renders at a fixed 80×24; a smaller local terminal will look
  clipped.

## Installation

### Released Binary

Install the latest server and client with one command:

```bash
curl -fsSL https://raw.githubusercontent.com/bscott/chat-tails/main/install/install.sh | bash
```

The installer selects the Linux or macOS binary for the current architecture,
verifies it against the release checksum, and installs it to
`~/.local/bin/chat-tails`. The same binary runs either mode:

```bash
# Start a server
chat-tails --history

# Connect with the built-in client
chat-tails --client --host chat.example.ts.net
```

Pin a specific release by setting `CHAT_TAILS_VERSION` on the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/bscott/chat-tails/main/install/install.sh | CHAT_TAILS_VERSION=v0.3.0 bash
```

### From Source

```bash
git clone https://github.com/bscott/chat-tails.git
cd chat-tails
make build
```

### Docker

```bash
# Build
docker build -t chat-tails .

# Run locally
docker run -p 2323:2323 chat-tails

# Run with Tailscale
docker run -e TS_AUTHKEY=tskey-auth-xxxxx -e TS_HOSTNAME=mychat chat-tails
```

### Docker Compose

The included `docker-compose.yml` runs the published image with the same
configuration variables:

```bash
# Start the latest release
docker compose up -d

# Follow server output, including the connection address
docker compose logs -f

# Enable Tailscale mode
TS_AUTHKEY=tskey-auth-xxxxx TS_HOSTNAME=mychat docker compose up -d

# Build the current checkout instead of using the published image
docker compose up -d --build
```

Set `CHAT_TAILS_VERSION` to pin a published image tag, such as `v0.3.0`.

Docker configuration uses dedicated environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 2323 | TCP port to listen on |
| `ROOM_NAME` | "Chat Room" | Name displayed in the chat |
| `MAX_USERS` | 10 | Maximum concurrent users |
| `TS_AUTHKEY` | empty | Enables Tailscale mode when set |
| `TS_HOSTNAME` | "chatroom" | Requested Tailscale node name |

`TS_HOSTNAME` is intentionally separate from Docker's reserved `HOSTNAME`
variable; `HOSTNAME` is ignored. The requested node name is not necessarily the
final MagicDNS name, so use the address printed after startup.

Docker Compose persists Tailscale identity in the `tsnet-state` volume so
container recreation does not register a new tailnet node.

## Configuration

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--port` | `-p` | 2323 | TCP port to listen on, or to connect to with `--client` (0 selects an ephemeral listen port) |
| `--client` | | false | Connect to a chat server instead of running one |
| `--host` | | | Server address to connect to (requires `--client`) |
| `--room-name` | `-r` | "Chat Room" | Name displayed in the chat |
| `--max-users` | `-m` | 10 | Maximum concurrent users |
| `--tailscale` | `-t` | false | Enable Tailscale mode |
| `--hostname` | `-H` | "chatroom" | Tailscale hostname (requires `--tailscale`) |
| `--history` | | false | Enable message history for new users |
| `--history-size` | | 50 | Number of messages to keep in history |
| `--plain-text` | | false | Disable ANSI formatting (for Windows telnet) |
| `--version` | `-v` | | Show version information |

## Windows Telnet Compatibility

Windows telnet has limited ANSI escape sequence support. If you see garbled formatting characters when connecting from Windows telnet, start the server with the `--plain-text` flag:

```bash
./chat-tails --plain-text
```

This disables all ANSI color codes and cursor control sequences for a better experience on legacy telnet clients.

**Recommended:** For the best experience on Windows, use a modern terminal emulator like:
- Windows Terminal with `telnet` or `ssh`
- PuTTY
- WSL with `nc` or `telnet`

## Tailscale Setup

1. Get an auth key from [Tailscale Admin Console](https://login.tailscale.com/admin/settings/keys)
2. Set the environment variable:
   ```bash
   export TS_AUTHKEY=tskey-auth-xxxxx
   ```
3. Run with Tailscale enabled:
   ```bash
   ./chat-tails --tailscale --hostname mychat
   ```
4. Copy the connection command printed after startup. It contains the actual
   MagicDNS name assigned by Tailscale; do not construct a `.ts.net` name from
   the requested hostname.

### Troubleshooting

If upgrading a Tailscale server from v0.2.x, move its state directory so the
renamed `chat-tails` binary keeps the existing tailnet identity:

```bash
# macOS
mv "$HOME/Library/Application Support/tsnet-chat-server" \
  "$HOME/Library/Application Support/tsnet-chat-tails"

# Linux
mv "$HOME/.config/tsnet-chat-server" "$HOME/.config/tsnet-chat-tails"
```

If you see "Authkey is set; but state is NoState":

```bash
# Option 1: Force new login
export TSNET_FORCE_LOGIN=1

# Option 2: Clear existing state
rm -rf "$HOME/Library/Application Support/tsnet-chat-tails"  # macOS
rm -rf "$HOME/.config/tsnet-chat-tails"                      # Linux
```

## Chat Commands

| Command | Description |
|---------|-------------|
| `/who` | List all users in the room |
| `/me <action>` | Send an action (e.g., `/me waves` → `* Brian waves`) |
| `/help` | Show available commands |
| `/quit` | Disconnect from chat |

## Development

```bash
# Build
make build

# Run tests
make test

# Run a single test
go test -v -run TestName ./internal/chat/

# Cross-compile for all platforms
make build-all
```

### Project Structure

```
├── cmd/chat-tails/    # Application entry point
├── internal/
│   ├── chat/          # Room and client handling
│   ├── server/        # Server lifecycle, Tailscale integration
│   └── ui/            # Terminal styling (lipgloss)
└── Makefile
```

## License

MIT
