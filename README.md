# Chat Tails

A terminal-based chat application built in Go. Share a chat room with friends over your Tailscale network - they connect via netcat or telnet, no client installation needed.

## Features

- **Tailscale Integration** - Share your chat server securely with anyone on your Tailnet
- **Zero Client Setup** - Users connect with just `nc` or `telnet`
- **Colorful UI** - Each user gets a unique color, styled messages with ANSI colors
- **Message History** - New users can see recent chat history (optional)
- **Chat Commands** - `/who`, `/me`, `/help`, `/quit`
- **Rate Limiting** - Built-in protection against spam

## Quick Start

```bash
# Build
make build

# Run locally
./chat-server

# Run with Tailscale (share with your network)
export TS_AUTHKEY=tskey-auth-xxxxx
./chat-server --tailscale --hostname mychat --history
```

Connect from any machine:
```bash
nc mychat.your-tailnet.ts.net 2323
```

## Installation

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
docker run -e TS_AUTHKEY=tskey-auth-xxxxx chat-tails --tailscale --hostname mychat
```

## Configuration

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--port` | `-p` | 2323 | TCP port to listen on |
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
./chat-server --plain-text
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
   ./chat-server --tailscale --hostname mychat
   ```
4. Share with others on your Tailnet - they connect with:
   ```bash
   nc mychat.your-tailnet.ts.net 2323
   ```

### Troubleshooting

If you see "Authkey is set; but state is NoState":
```bash
# Option 1: Force new login
export TSNET_FORCE_LOGIN=1

# Option 2: Clear existing state
rm -rf ~/Library/Application\ Support/tsnet-chat-server/  # macOS
rm -rf ~/.local/share/tsnet-chat-server/                  # Linux
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
