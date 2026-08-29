package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"golang.org/x/term"
)

func TestValidateClientFlags(t *testing.T) {
	none := func(string) bool { return false }
	tests := []struct {
		name    string
		opts    clientOptions
		port    int
		changed func(string) bool
		wantErr string
	}{
		{name: "server mode", opts: clientOptions{}, port: defaultPort, changed: none},
		{name: "client with host", opts: clientOptions{Enabled: true, Host: "chat.example.ts.net"}, port: defaultPort, changed: none},
		{
			name:    "client without host",
			opts:    clientOptions{Enabled: true},
			port:    defaultPort,
			wantErr: "--client requires --host",
		},
		{
			name:    "host without client",
			opts:    clientOptions{Host: "chat.example.ts.net"},
			port:    defaultPort,
			wantErr: "--host requires --client",
		},
		{
			name:    "client with server flag",
			opts:    clientOptions{Enabled: true, Host: "chat.example.ts.net"},
			port:    defaultPort,
			changed: func(flag string) bool { return flag == "tailscale" },
			wantErr: "--tailscale is a server flag",
		},
		{
			name:    "client with plain-text flag",
			opts:    clientOptions{Enabled: true, Host: "localhost"},
			port:    defaultPort,
			changed: func(flag string) bool { return flag == "plain-text" },
			wantErr: "--plain-text is a server flag",
		},
		{
			name:    "client with zero port",
			opts:    clientOptions{Enabled: true, Host: "localhost"},
			port:    0,
			changed: none,
			wantErr: "--port must be between 1 and 65535",
		},
		{
			name:    "client with oversized port",
			opts:    clientOptions{Enabled: true, Host: "localhost"},
			port:    65536,
			changed: none,
			wantErr: "--port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateClientFlags(tt.opts, tt.port, tt.changed)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateClientFlags: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateClientFlags error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestDefineFlagsParsesClientInvocation(t *testing.T) {
	flags := pflag.NewFlagSet("chat-tails", pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var cfg config
	var opts clientOptions
	var showVersion bool
	defineFlags(flags, &cfg, &opts, &showVersion)

	if err := flags.Parse([]string{"--client", "--host", "chat.example.ts.net", "--port", "4242"}); err != nil {
		t.Fatalf("parse client flags: %v", err)
	}
	if !opts.Enabled || opts.Host != "chat.example.ts.net" || cfg.Port != 4242 {
		t.Fatalf("parsed client flags = opts %#v, port %d", opts, cfg.Port)
	}
	if err := validateClientFlags(opts, cfg.Port, flags.Changed); err != nil {
		t.Fatalf("validate parsed client flags: %v", err)
	}
}

// recordingTerminal counts raw-mode transitions through the injectable boundary.
type recordingTerminal struct {
	rawCalls     int
	restoreCalls int
	makeRawErr   error
	restoreErr   error
}

func (r *recordingTerminal) control(isTerminal bool) terminalControl {
	return terminalControl{
		fd:         42,
		isTerminal: func(int) bool { return isTerminal },
		makeRaw: func(int) (*term.State, error) {
			r.rawCalls++
			if r.makeRawErr != nil {
				return nil, r.makeRawErr
			}
			return &term.State{}, nil
		},
		restore: func(int, *term.State) error {
			r.restoreCalls++
			return r.restoreErr
		},
	}
}

func TestEnableRawInputRestoresExactlyOnce(t *testing.T) {
	recorder := &recordingTerminal{}
	restore := enableRawInput(recorder.control(true), &bytes.Buffer{})
	if recorder.rawCalls != 1 {
		t.Fatalf("makeRaw calls = %d, want 1", recorder.rawCalls)
	}

	restore()
	restore() // signal path and defer path may both fire; must stay idempotent
	if recorder.restoreCalls != 1 {
		t.Fatalf("restore calls = %d, want exactly 1", recorder.restoreCalls)
	}
}

func TestEnableRawInputSkipsNonTerminal(t *testing.T) {
	recorder := &recordingTerminal{}
	restore := enableRawInput(recorder.control(false), &bytes.Buffer{})
	restore()
	if recorder.rawCalls != 0 || recorder.restoreCalls != 0 {
		t.Fatalf("non-terminal input touched the terminal: raw=%d restore=%d", recorder.rawCalls, recorder.restoreCalls)
	}
}

func TestEnableRawInputHandlesMakeRawFailure(t *testing.T) {
	recorder := &recordingTerminal{makeRawErr: errors.New("no tty")}
	var warnings bytes.Buffer
	restore := enableRawInput(recorder.control(true), &warnings)
	restore()
	if recorder.restoreCalls != 0 {
		t.Fatalf("restore called %d times after failed makeRaw, want 0", recorder.restoreCalls)
	}
	if !strings.Contains(warnings.String(), "raw terminal mode") {
		t.Fatalf("missing raw-mode warning, got %q", warnings.String())
	}
}

func TestEnableRawInputWarnsOnRestoreFailure(t *testing.T) {
	recorder := &recordingTerminal{restoreErr: errors.New("tty gone")}
	var warnings bytes.Buffer
	restore := enableRawInput(recorder.control(true), &warnings)
	restore()
	if !strings.Contains(warnings.String(), "failed to restore terminal") {
		t.Fatalf("missing restore warning, got %q", warnings.String())
	}
}
