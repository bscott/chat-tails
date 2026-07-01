package ui

import (
	"strings"
	"testing"
)

// Tests for the styled (lipgloss) formatters. lipgloss disables color when it
// detects a non-TTY / no-color environment, so these tests assert on content
// and determinism rather than exact ANSI bytes.

// --- Unit -----------------------------------------------------------------

// TestUnit_GetUserColorDeterministic verifies the same username always maps to
// the same color and that the result is one of the defined palette entries.
func TestUnit_GetUserColorDeterministic(t *testing.T) {
	for _, name := range []string{"alice", "bob", "", "a-very-long-nickname"} {
		first := GetUserColor(name)
		if first != GetUserColor(name) {
			t.Errorf("GetUserColor(%q) is not deterministic", name)
		}
		found := false
		for _, c := range UserColors {
			if c == first {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GetUserColor(%q)=%q not in the defined palette", name, first)
		}
	}
}

// TestUnit_GetUserColorNeverPanicsOnUnicode guards the hash arithmetic against
// multi-byte runes and negative-hash handling.
func TestUnit_GetUserColorNeverPanicsOnUnicode(t *testing.T) {
	got := GetUserColor("naïve-用户-🙂")
	if got == "" {
		t.Error("expected a non-empty color for a unicode nickname")
	}
}

// TestUnit_FormatHelpContainsAllCommands checks the styled help still lists
// every command regardless of styling.
func TestUnit_FormatHelpContainsAllCommands(t *testing.T) {
	got := FormatHelp()
	for _, cmd := range []string{"/who", "/me", "/help", "/quit"} {
		if !strings.Contains(got, cmd) {
			t.Errorf("styled help missing %q; got:\n%s", cmd, got)
		}
	}
}

// --- Functional -----------------------------------------------------------

// TestFunctional_StyledFormattersIncludePayload verifies each styled formatter
// preserves the caller's message/username content (styling must not drop text).
func TestFunctional_StyledFormattersIncludePayload(t *testing.T) {
	if !strings.Contains(FormatSystemMessage("ping"), "ping") {
		t.Error("system message dropped payload")
	}
	if !strings.Contains(FormatUserMessage("alice", "hello", "12:00"), "hello") {
		t.Error("user message dropped payload")
	}
	if !strings.Contains(FormatSelfMessage("mine", "12:00"), "mine") {
		t.Error("self message dropped payload")
	}
	if !strings.Contains(FormatActionMessage("alice", "waves"), "waves") {
		t.Error("action message dropped payload")
	}
	if !strings.Contains(FormatTitle("Lobby"), "Lobby") {
		t.Error("title dropped payload")
	}
	welcome := FormatWelcomeMessage("lobby", "alice")
	if !strings.Contains(welcome, "lobby") || !strings.Contains(welcome, "alice") {
		t.Error("welcome dropped payload")
	}
}

// TestFunctional_FormatUserListCountsUsers verifies the styled user list renders
// the correct occupancy and every member name.
func TestFunctional_FormatUserListCountsUsers(t *testing.T) {
	got := FormatUserList("lobby", []string{"alice", "bob", "carol"}, 8)
	if !strings.Contains(got, "3/8") {
		t.Errorf("expected 3/8 occupancy, got:\n%s", got)
	}
	for _, u := range []string{"alice", "bob", "carol"} {
		if !strings.Contains(got, u) {
			t.Errorf("user list missing %q; got:\n%s", u, got)
		}
	}
}

// --- Performance ----------------------------------------------------------

// TestPerformance_CreateColoredBoxLargeContent ensures box rendering handles a
// large body without error.
func TestPerformance_CreateColoredBoxLargeContent(t *testing.T) {
	body := strings.Repeat("line of chat\n", 500)
	if out := CreateColoredBox("Title", body, 40); out == "" {
		t.Error("expected non-empty rendered box")
	}
}

// --- Security / Retry / Integration / Frame -------------------------------

// TestPlaceholders_StyledCategories documents categories that are exercised by
// sibling files or do not apply to the pure styling helpers:
//   - Security    : covered by plaintext_test.go (plain mode is the untrusted path)
//   - Retry       : N/A — pure functions, nothing to retry
//   - Integration : covered by plaintext_test.go (plain/styled help parity)
//   - Frame       : N/A — framing lives in the server layer
func TestPlaceholders_StyledCategories(t *testing.T) {
	t.Skip("N/A / covered elsewhere: see doc comment for the coverage matrix")
}
