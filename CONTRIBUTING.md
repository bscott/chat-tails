# Contributing to Chat Tails

Thanks for helping improve Chat Tails. This guide describes the current local
workflow and repository conventions.

## Setup

1. Fork the repository and clone your fork.
2. Install Go 1.24.2 or later.
3. Confirm the project builds and its tests pass:

   ```bash
   make build
   make test
   ```

The application entry point is `cmd/chat-tails/main.go`.

## Development workflow

Create a focused topic branch from `main`. Before opening a pull request, run:

```bash
gofmt -w path/to/changed.go
go vet ./...
make test
make build
```

Format the Go files you changed rather than the entire repository. `make test`
runs `go test -v ./...`, matching the package scope used by CI.

For pull requests that change Go code, dependencies, build files, Docker files,
or workflows, CI verifies dependencies, builds the application, runs `go vet`
and `make test`, and performs a multi-architecture Docker build. Documentation-
only changes may not start that workflow.

## Repository structure

- `cmd/chat-tails` parses command-line flags and manages process startup and
  shutdown.
- `internal/server` owns TCP and Tailscale listeners, connections, and server
  lifecycle.
- `internal/chat` owns rooms, clients, commands, rate limiting, history, and the
  Bubble Tea model.
- `internal/ui` contains styled and plain-text terminal formatters.
- `install` contains the Docker and systemd entrypoints.

The room processes joins, leaves, and broadcasts through an event loop. Shared
maps and per-client writers are additionally protected by mutexes. Server and
client shutdown is coordinated with contexts.

The application supports two client interfaces: the default Bubble Tea TUI and
the `--plain-text` line-oriented interface. Keep behavior consistent between
both paths. In particular:

- Update both command handlers when adding a chat command.
- Update both styled and plain-text help output.
- Provide styled and plain-text variants for user-facing formatters.

## Tests

Tests should defend observable behavior and fail for a plausible regression.
Prefer deterministic assertions around boundaries such as protocol input and
output, connection lifecycle, command behavior, formatting, and configuration
flow. Use real local TCP connections for server integration behavior where that
is the contract under test.

Do not add skipped placeholders for test categories that do not apply. A test
name and its comments should describe only behavior the test actually checks.

Run all tests with:

```bash
make test
```

Run a package or individual test with standard Go tooling, for example:

```bash
go test ./internal/server
go test -run TestPlainTextProtocolAndConnectionCleanup ./internal/server
```

## Pull requests

- Keep each pull request focused on one concern.
- Explain what changed and why.
- Include verification commands and results.
- Add or update tests when observable behavior changes.
- Link related issues when applicable.
- Use concise, imperative commit subjects.

## Reporting issues

For bugs, include reproduction steps, operating system, Go version, and whether
the server was using regular TCP or Tailscale mode. For feature requests,
describe the use case and expected behavior.

## License

Contributions are licensed under the repository's [MIT License](LICENSE).
