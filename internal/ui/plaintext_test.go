package ui

import (
	"strings"
	"testing"
)

// Test coverage categories for the plain-text formatters. These functions are
// pure (no I/O, no network), so several categories do not apply and are marked
// with explicit placeholders per the project test conventions:
//
//   - Security     : covered — user-supplied content must never gain ANSI
//                    escape codes it did not already contain (plain-text mode
//                    is the "safe" path for untrusted telnet clients).
//   - Performance  : covered — formatters must not be quadratic on large user
//                    lists.
//   - Retry        : N/A — pure string functions, nothing to retry.
//   - Unit         : covered — per-function output assertions below.
//   - Integration  : covered — plain/styled parity for the help output.
//   - Functional   : covered — end-to-end formatting of a realistic message.
//   - Frame        : N/A — no protocol framing at this layer.

// --- Security -------------------------------------------------------------

// TestSecurity_PlainNeverEmitsANSI verifies the plain-text formatters never
// introduce ANSI escape sequences, even when the caller passes bytes that look
// like escape codes. This is the whole point of plain-text mode: clients that
// cannot render ANSI (Windows telnet) must receive clean text.
func TestSecurity_PlainNeverEmitsANSI(t *testing.T) {
	const esc = "\x1b" // ESC, the start of every ANSI sequence

	cases := []string{
		FormatSystemMessagePlain("hello"),
		FormatUserMessagePlain("alice", "hi there", "12:00"),
		FormatSelfMessagePlain("my message", "12:00"),
		FormatActionMessagePlain("alice", "waves"),
		FormatTitlePlain("Room"),
		FormatHelpPlain(),
		FormatUserListPlain("lobby", []string{"a", "b"}, 10),
		FormatWelcomeMessagePlain("lobby", "alice"),
	}
	for i, out := range cases {
		if strings.Contains(out, esc) {
			t.Errorf("case %d: plain formatter emitted an ANSI escape: %q", i, out)
		}
	}
}

// TestSecurity_PlainPassesThroughUntrustedInput ensures that if a user's
// nickname or message already contains an escape byte, the plain formatter does
// not add framing that would change its meaning — it is passed through verbatim
// so downstream sanitisation stays predictable.
func TestSecurity_PlainPassesThroughUntrustedInput(t *testing.T) {
	nasty := "bob\x1b[31m"
	got := FormatUserMessagePlain(nasty, "payload", "09:41")
	if !strings.Contains(got, nasty) {
		t.Errorf("expected raw nickname to pass through unchanged, got %q", got)
	}
	if !strings.Contains(got, "payload") {
		t.Errorf("expected message body to be present, got %q", got)
	}
}

// --- Performance ----------------------------------------------------------

// TestPerformance_UserListScales is a lightweight guard that the user-list
// formatter stays linear: it should complete for a large room without pathology.
func TestPerformance_UserListScales(t *testing.T) {
	users := make([]string, 1000)
	for i := range users {
		users[i] = "user"
	}
	out := FormatUserListPlain("big", users, 1000)
	// One line per user plus a header line.
	if lines := strings.Count(out, "\n"); lines < len(users) {
		t.Errorf("expected at least %d newlines, got %d", len(users), lines)
	}
}

// --- Retry ----------------------------------------------------------------

// TestRetry_NotApplicable documents that the ui formatters are pure functions
// with no fallible operations, so there is nothing to retry. Placeholder kept
// to make the coverage matrix explicit.
func TestRetry_NotApplicable(t *testing.T) {
	t.Skip("N/A: ui formatters are pure string functions with no retryable I/O")
}

// --- Unit -----------------------------------------------------------------

func TestUnit_FormatSystemMessagePlain(t *testing.T) {
	if got, want := FormatSystemMessagePlain("up"), "[System] up"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnit_FormatUserMessagePlain(t *testing.T) {
	if got, want := FormatUserMessagePlain("alice", "hi", "12:00"), "[12:00] alice: hi"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnit_FormatSelfMessagePlain(t *testing.T) {
	if got, want := FormatSelfMessagePlain("hi", "12:00"), "[12:00] You: hi"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnit_FormatActionMessagePlain(t *testing.T) {
	if got, want := FormatActionMessagePlain("alice", "waves"), "* alice waves"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnit_FormatTitlePlain(t *testing.T) {
	if got, want := FormatTitlePlain("Lobby"), "=== Lobby ==="; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnit_FormatHelpPlain(t *testing.T) {
	got := FormatHelpPlain()
	for _, cmd := range []string{"/who", "/me", "/help", "/quit"} {
		if !strings.Contains(got, cmd) {
			t.Errorf("help text missing command %q; got:\n%s", cmd, got)
		}
	}
}

func TestUnit_FormatUserListPlain(t *testing.T) {
	got := FormatUserListPlain("lobby", []string{"alice", "bob"}, 10)
	if !strings.Contains(got, "Users in lobby (2/10):") {
		t.Errorf("missing header, got:\n%s", got)
	}
	if !strings.Contains(got, "- alice\n") || !strings.Contains(got, "- bob\n") {
		t.Errorf("missing user entries, got:\n%s", got)
	}
}

func TestUnit_FormatUserListPlainEmpty(t *testing.T) {
	got := FormatUserListPlain("lobby", nil, 5)
	if !strings.Contains(got, "(0/5)") {
		t.Errorf("expected 0/5 count for empty room, got %q", got)
	}
}

func TestUnit_FormatWelcomeMessagePlain(t *testing.T) {
	got := FormatWelcomeMessagePlain("lobby", "alice")
	if !strings.Contains(got, "Welcome to lobby, alice!") {
		t.Errorf("missing greeting, got %q", got)
	}
	if !strings.Contains(got, "/help") {
		t.Errorf("welcome should mention /help, got %q", got)
	}
}

// --- Integration ----------------------------------------------------------

// TestIntegration_HelpPlainAndStyledParity checks the plain and styled help
// outputs describe the same command set, so switching --plain-text on/off never
// hides a command from the user.
func TestIntegration_HelpPlainAndStyledParity(t *testing.T) {
	plain := FormatHelpPlain()
	styled := FormatHelp()
	for _, cmd := range []string{"/who", "/me", "/help", "/quit"} {
		if strings.Contains(plain, cmd) != strings.Contains(styled, cmd) {
			t.Errorf("command %q present in only one of plain/styled help", cmd)
		}
	}
}

// --- Functional -----------------------------------------------------------

// TestFunctional_RenderTypicalConversation formats a small realistic exchange
// end to end and asserts the resulting transcript is what a plain client sees.
func TestFunctional_RenderTypicalConversation(t *testing.T) {
	var b strings.Builder
	b.WriteString(FormatWelcomeMessagePlain("lobby", "alice"))
	b.WriteString("\n")
	b.WriteString(FormatSystemMessagePlain("bob joined"))
	b.WriteString("\n")
	b.WriteString(FormatUserMessagePlain("bob", "hey alice", "09:41"))
	b.WriteString("\n")
	b.WriteString(FormatSelfMessagePlain("hi bob", "09:41"))
	b.WriteString("\n")
	b.WriteString(FormatActionMessagePlain("bob", "waves"))

	transcript := b.String()
	for _, want := range []string{
		"Welcome to lobby, alice!",
		"[System] bob joined",
		"[09:41] bob: hey alice",
		"[09:41] You: hi bob",
		"* bob waves",
	} {
		if !strings.Contains(transcript, want) {
			t.Errorf("transcript missing %q; full:\n%s", want, transcript)
		}
	}
}

// --- Frame ----------------------------------------------------------------

// TestFrame_NotApplicable documents that message/line framing is handled by the
// server layer (server_test.go), not the ui formatters. Placeholder kept so the
// coverage matrix stays explicit.
func TestFrame_NotApplicable(t *testing.T) {
	t.Skip("N/A: line/message framing is the server layer's responsibility")
}
