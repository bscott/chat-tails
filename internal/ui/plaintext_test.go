package ui

import (
	"reflect"
	"strings"
	"testing"
)

func TestPlainFormatters(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "system message", got: FormatSystemMessagePlain("server ready"), want: "[System] server ready"},
		{name: "user message", got: FormatUserMessagePlain("alice", "hello", "12:34:56"), want: "[12:34:56] alice: hello"},
		{name: "self message", got: FormatSelfMessagePlain("hello", "12:34:56"), want: "[12:34:56] You: hello"},
		{name: "action message", got: FormatActionMessagePlain("alice", "waves"), want: "* alice waves"},
		{name: "title", got: FormatTitlePlain("Lobby"), want: "=== Lobby ==="},
		{
			name: "user list",
			got:  FormatUserListPlain("Lobby", []string{"alice", "bob"}, 10),
			want: "Users in Lobby (2/10):\n- alice\n- bob\n",
		},
		{name: "empty user list", got: FormatUserListPlain("Lobby", nil, 10), want: "Users in Lobby (0/10):\n"},
		{
			name: "welcome message",
			got:  FormatWelcomeMessagePlain("Lobby", "alice"),
			want: "Welcome to Lobby, alice!\n\nType a message and press Enter to send. Use /help to see available commands.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("output = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestPlainFormattersDoNotIntroduceANSIForPlainContent(t *testing.T) {
	outputs := []string{
		FormatSystemMessagePlain("hello"),
		FormatUserMessagePlain("alice", "hello", "12:34:56"),
		FormatSelfMessagePlain("hello", "12:34:56"),
		FormatActionMessagePlain("alice", "waves"),
		FormatTitlePlain("Lobby"),
		FormatHelpPlain(),
		FormatUserListPlain("Lobby", []string{"alice"}, 10),
		FormatWelcomeMessagePlain("Lobby", "alice"),
	}

	for i, output := range outputs {
		if strings.ContainsRune(output, '\x1b') {
			t.Errorf("output %d added an ANSI escape: %q", i, output)
		}
	}
}

func TestPlainFormattersPreserveUnicodeContent(t *testing.T) {
	got := FormatUserMessagePlain("josé", "hello 世界", "09:41")
	if want := "[09:41] josé: hello 世界"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestPlainAndStyledHelpListTheSameCommands(t *testing.T) {
	plain := extractCommands(FormatHelpPlain())
	styled := extractCommands(FormatHelp())
	want := map[string]struct{}{`/who`: {}, `/me`: {}, `/help`: {}, `/quit`: {}}

	if !reflect.DeepEqual(plain, want) {
		t.Errorf("plain help commands = %v, want %v", plain, want)
	}
	if !reflect.DeepEqual(styled, want) {
		t.Errorf("styled help commands = %v, want %v", styled, want)
	}
}

func extractCommands(value string) map[string]struct{} {
	commands := make(map[string]struct{})
	for i := 0; i < len(value); i++ {
		if value[i] != '/' {
			continue
		}
		end := i + 1
		for end < len(value) && value[end] >= 'a' && value[end] <= 'z' {
			end++
		}
		if end > i+1 {
			commands[value[i:end]] = struct{}{}
		}
		i = end - 1
	}
	return commands
}
