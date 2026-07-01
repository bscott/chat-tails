# Contributing to Chat Tails

Thanks for your interest in improving Chat Tails! This document covers how to
get set up, the conventions the project follows, and what to expect when you
open a pull request.

## Getting started

1. Fork the repository and clone your fork.
2. Make sure you have **Go 1.24+** installed (`go version`).
3. Build and run the tests to confirm your environment is working:

   ```bash
   make build
   make test
   ```

The entry point is `cmd/chat-tails/main.go`. See `CLAUDE.md` for a tour of the
package layout (`internal/chat`, `internal/server`, `internal/ui`) and the key
concurrency patterns.

## Development workflow

- Create a topic branch off `main` for your change.
- Keep each pull request focused on a single concern.
- Before pushing, run the full local check that CI runs:

  ```bash
  go build ./...
  go vet ./...
  go test ./...      # or: make test
  ```

  CI (`.github/workflows/test-build.yml`) runs `go build`, `go test -v ./...`,
  and a multi-arch Docker build on every pull request, so running these locally
  first saves a round trip.

## Coding conventions

- Format all code with `gofmt` (run `gofmt -w .` or configure your editor to
  format on save). CI-clean code is `gofmt`-clean code.
- Fix everything `go vet ./...` reports.
- Follow the existing package boundaries:
  - `internal/chat` — core chat logic (rooms, clients, messages).
  - `internal/server` — server lifecycle, connection handling, Tailscale.
  - `internal/ui` — terminal styling (lipgloss) and plain-text formatters.
- When you add a user-facing formatter, provide **both** a styled variant and a
  plain-text variant so `--plain-text` clients (e.g. Windows telnet) stay in
  sync.

## Tests

New behavior should come with tests. Where practical, organize tests against the
project's standard coverage categories, and use a skipped placeholder with a
short comment when a category genuinely does not apply:

- **Security** — input handling, escaping, resource release.
- **Performance** — behavior stays reasonable under load / large inputs.
- **Retry** — retryable operations back off / recover (N/A for pure functions).
- **Unit** — individual functions in isolation.
- **Integration** — components working together (e.g. start → connect → stop).
- **Functional** — realistic end-to-end scenarios.
- **Frame** — protocol/line framing where relevant.

Run tests with `make test` (which runs `go test -v ./...`).

## Commit messages

- Write a concise, imperative subject line (e.g. "Add plain-text welcome
  formatter").
- Explain the *why* in the body when the change isn't obvious.

## Opening a pull request

- Describe what the change does and why.
- Confirm `go build ./...`, `go vet ./...`, and `go test ./...` all pass.
- Link any related issue.

## Reporting bugs and requesting features

Open an issue with clear steps to reproduce (for bugs) or a description of the
use case (for features). Include your OS, Go version, and whether you're running
in TCP or Tailscale mode when relevant.

## License

By contributing, you agree that your contributions will be licensed under the
[MIT License](LICENSE) that covers this project.
