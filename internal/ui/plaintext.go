package ui

import "fmt"

// PlainText formatters for Windows telnet and other clients with limited ANSI support

// FormatSystemMessagePlain formats a system message without ANSI codes
func FormatSystemMessagePlain(message string) string {
	return "[System] " + message
}

// FormatUserMessagePlain formats a user message without ANSI codes
func FormatUserMessagePlain(username, message, timestamp string) string {
	return "[" + timestamp + "] " + username + ": " + message
}

// FormatSelfMessagePlain formats the user's own message without ANSI codes
func FormatSelfMessagePlain(message, timestamp string) string {
	return "[" + timestamp + "] You: " + message
}

// FormatActionMessagePlain formats an action message without ANSI codes
func FormatActionMessagePlain(username, action string) string {
	return "* " + username + " " + action
}

// FormatTitlePlain formats a title without ANSI codes
func FormatTitlePlain(title string) string {
	return "=== " + title + " ==="
}

// FormatHelpPlain formats the help message without ANSI codes
func FormatHelpPlain() string {
	return `
Available Commands:
  /who - Show all users in the room
  /me <action> - Perform an action
  /help - Show this help message
  /quit - Leave the chat
`
}

// FormatUserListPlain formats the user list without ANSI codes
func FormatUserListPlain(roomName string, users []string, maxUsers int) string {
	content := fmt.Sprintf("Users in %s (%d/%d):\n", roomName, len(users), maxUsers)
	for _, user := range users {
		content += "- " + user + "\n"
	}
	return content
}

// FormatWelcomeMessagePlain formats the welcome message without ANSI codes
func FormatWelcomeMessagePlain(roomName, nickname string) string {
	return fmt.Sprintf("Welcome to %s, %s!\n\nType a message and press Enter to send. Use /help to see available commands.", roomName, nickname)
}
