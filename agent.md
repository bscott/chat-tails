# Agent Guidelines

## Project Context

Terminal chat server in Go with optional Tailscale networking. Users connect via telnet/netcat.

## Quick Reference

| Task | Command |
|------|---------|
| Build | `make build` |
| Run | `make run` or `./chat-tails` |
| Test | `make test` |
| Single test | `go test -v -run TestName ./internal/chat/` |

## Code Locations

- Entry point: `cmd/chat-tails/main.go`
- Server lifecycle: `internal/server/server.go`
- Chat room logic: `internal/chat/room.go`
- Client handling: `internal/chat/client.go`
- UI styling: `internal/ui/styles.go`

## Implementation Notes

### Adding New Chat Commands

1. Add case to `handleCommand()` in `internal/chat/client.go`
2. Update help text in `internal/ui/styles.go:FormatHelp()`

### Modifying Room Behavior

Room uses channel-based event loop in `run()`. Client map operations go through `join`/`leave` channels to avoid races. Don't access `r.clients` directly outside of `addClient`/`removeClient`.

### Adding New Config Options

1. Add field to `internal/server/config.go:Config`
2. Add flag in `cmd/chat-tails/main.go:parseFlags()`
3. Use in `internal/server/server.go`

## Testing

Tests exist for `internal/chat/` package. Run with `make test` or target specific tests:

```bash
go test -v -run TestRoom ./internal/chat/
go test -v -run TestClient ./internal/chat/
```

## Dependencies

- `github.com/charmbracelet/lipgloss` - Terminal styling
- `github.com/spf13/pflag` - CLI flags
- `tailscale.com/tsnet` - Tailscale integration
