package ui

import (
	"strings"
	"testing"
)

func TestGetUserColorIsDeterministicAndUsesPalette(t *testing.T) {
	for _, username := range []string{"alice", "bob", "carol", "user-20"} {
		got := GetUserColor(username)
		if again := GetUserColor(username); got != again {
			t.Errorf("GetUserColor(%q) = %q then %q", username, got, again)
		}
		if !contains(UserColors, got) {
			t.Errorf("GetUserColor(%q) = %q, not in palette %v", username, got, UserColors)
		}
	}
}

func TestStyledFormattersPreserveContent(t *testing.T) {
	tests := []struct {
		name  string
		got   string
		parts []string
	}{
		{name: "system message", got: FormatSystemMessage("server ready"), parts: []string{"System", "server ready"}},
		{name: "user message", got: FormatUserMessage("alice", "hello", "12:34:56"), parts: []string{"12:34:56", "alice", "hello"}},
		{name: "self message", got: FormatSelfMessage("hello", "12:34:56"), parts: []string{"12:34:56", "You", "hello"}},
		{name: "action message", got: FormatActionMessage("alice", "waves"), parts: []string{"alice", "waves"}},
		{name: "title", got: FormatTitle("Lobby"), parts: []string{"Lobby"}},
		{name: "welcome", got: FormatWelcomeMessage("Lobby", "alice"), parts: []string{"Lobby", "alice", "/help"}},
		{name: "colored box", got: CreateColoredBox("Notice", "body text", 40), parts: []string{"Notice", "body text"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, part := range tt.parts {
				if !strings.Contains(tt.got, part) {
					t.Errorf("output %q does not contain %q", tt.got, part)
				}
			}
		})
	}
}

func TestFormatUserListShowsOccupancyAndUsers(t *testing.T) {
	got := FormatUserList("Lobby", []string{"alice", "bob", "carol"}, 8)
	for _, part := range []string{"Lobby", "3/8", "alice", "bob", "carol"} {
		if !strings.Contains(got, part) {
			t.Errorf("output %q does not contain %q", got, part)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
