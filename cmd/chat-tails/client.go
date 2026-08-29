package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"golang.org/x/term"

	"github.com/bscott/ts-chat/internal/client"
)

// clientOptions holds the flags that select and configure client mode.
type clientOptions struct {
	Enabled bool
	Host    string
}

// serverOnlyFlags cannot be combined with --client.
var serverOnlyFlags = []string{
	"tailscale", "hostname", "room-name", "max-users", "history", "history-size", "plain-text",
}

// validateClientFlags rejects incomplete or contradictory client-mode flags.
// changed reports whether a named flag was set explicitly.
func validateClientFlags(opts clientOptions, port int, changed func(string) bool) error {
	if !opts.Enabled {
		if opts.Host != "" {
			return fmt.Errorf("--host requires --client")
		}
		return nil
	}
	if opts.Host == "" {
		return fmt.Errorf("--client requires --host <server address>")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("--port must be between 1 and 65535 in client mode")
	}
	for _, flag := range serverOnlyFlags {
		if changed(flag) {
			return fmt.Errorf("--%s is a server flag and cannot be combined with --client", flag)
		}
	}
	return nil
}

// runClient connects to host:port and relays the server-rendered TUI to the
// local terminal. It returns the process exit code.
func runClient(host string, port int) int {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.Dial("tcp", address)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chat-tails: cannot connect to %s: %v\n", address, err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	restore := enableRawInput(stdinTerminalControl(), os.Stderr)
	defer restore()

	runErr := client.Run(ctx, client.Config{Conn: conn, In: os.Stdin, Out: os.Stdout})
	restore()

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "chat-tails: %v\n", runErr)
		return 1
	}
	fmt.Println("Disconnected.")
	return 0
}

// terminalControl abstracts raw-mode handling so tests can observe that the
// terminal is always restored.
type terminalControl struct {
	fd         int
	isTerminal func(int) bool
	makeRaw    func(int) (*term.State, error)
	restore    func(int, *term.State) error
}

func stdinTerminalControl() terminalControl {
	return terminalControl{
		fd:         int(os.Stdin.Fd()),
		isTerminal: term.IsTerminal,
		makeRaw:    term.MakeRaw,
		restore:    term.Restore,
	}
}

// enableRawInput switches the local terminal to raw mode and returns an
// idempotent restore function. It is a no-op when stdin is not a terminal,
// so piped input (and tests) never touch terminal state.
func enableRawInput(tc terminalControl, warn io.Writer) func() {
	if !tc.isTerminal(tc.fd) {
		return func() {}
	}

	state, err := tc.makeRaw(tc.fd)
	if err != nil {
		fmt.Fprintf(warn, "chat-tails: cannot enable raw terminal mode: %v\n", err)
		return func() {}
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			if err := tc.restore(tc.fd, state); err != nil {
				fmt.Fprintf(warn, "chat-tails: failed to restore terminal: %v\n", err)
			}
		})
	}
}
