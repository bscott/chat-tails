package ui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// SanitizeTerminalText removes terminal controls from an untrusted operand.
func SanitizeTerminalText(value string) string {
	needsSanitizing := !utf8.ValidString(value)
	if !needsSanitizing {
		for _, r := range value {
			if r != '\n' && unicode.IsControl(r) {
				needsSanitizing = true
				break
			}
		}
	}
	if !needsSanitizing {
		return value
	}

	var sanitized strings.Builder
	sanitized.Grow(len(value))
	for _, r := range value {
		if r == '\n' || !unicode.IsControl(r) {
			sanitized.WriteRune(r)
		}
	}
	return sanitized.String()
}

// Color definitions
var (
	subtle    = lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight = lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special   = lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#2B5F3A"}
	accent    = lipgloss.AdaptiveColor{Light: "#1D9BF0", Dark: "#1D9BF0"}
	warning   = lipgloss.AdaptiveColor{Light: "#F25D94", Dark: "#F25D94"}
)

// UserColors is a list of colors for different users
var UserColors = []string{
	"#1D9BF0", // Blue
	"#F25D94", // Pink
	"#43BF6D", // Green
	"#FF6B35", // Orange
	"#9B5DE5", // Purple
	"#00F5D4", // Cyan
	"#FEE440", // Yellow
	"#FF595E", // Red
}

// GetUserColor returns a consistent color for a username based on hash
func GetUserColor(username string) string {
	hash := 0
	for _, c := range username {
		hash = int(c) + ((hash << 5) - hash)
	}
	if hash < 0 {
		hash = -hash
	}
	return UserColors[hash%len(UserColors)]
}

// Style definitions
var (
	// Base styles
	BaseStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(subtle)

	// Headers
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight).
			Padding(0, 1)

	// Text styles
	SystemStyle = lipgloss.NewStyle().
			Foreground(special).
			Bold(true)

	UserStyle = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)

	SelfStyle = lipgloss.NewStyle().
			Foreground(highlight).
			Bold(true)

	ActionStyle = lipgloss.NewStyle().
			Foreground(warning).
			Italic(true)

	// UI components
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(subtle).
			Padding(0, 1)

	InputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(highlight).
			Padding(0, 1)
)

// FormatSystemMessage formats a system message
func FormatSystemMessage(message string) string {
	return SystemStyle.Render("[System] " + SanitizeTerminalText(message))
}

// FormatUserMessage formats a user message
func FormatUserMessage(username, message, timestamp string) string {
	username = SanitizeTerminalText(username)
	message = SanitizeTerminalText(message)
	timestamp = SanitizeTerminalText(timestamp)
	userColor := GetUserColor(username)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(userColor)).Bold(true)
	return style.Render("["+timestamp+"] "+username+": ") + message
}

// FormatSelfMessage formats the user's own message
func FormatSelfMessage(message, timestamp string) string {
	message = SanitizeTerminalText(message)
	timestamp = SanitizeTerminalText(timestamp)
	return SelfStyle.Render("["+timestamp+"] You: ") + message
}

// FormatActionMessage formats an action message
func FormatActionMessage(username, action string) string {
	username = SanitizeTerminalText(username)
	action = SanitizeTerminalText(action)
	userColor := GetUserColor(username)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(userColor)).Italic(true)
	return style.Render("* " + username + " " + action)
}

// FormatTitle formats a title
func FormatTitle(title string) string {
	return HeaderStyle.Render("=== " + SanitizeTerminalText(title) + " ===")
}

// CreateColoredBox creates a colored box with a title and content
func CreateColoredBox(title, content string, width int) string {
	box := BoxStyle.Width(width)
	return box.Render(
		HeaderStyle.Render(SanitizeTerminalText(title)) + "\n\n" +
			SanitizeTerminalText(content),
	)
}

// FormatHelp formats the help message
func FormatHelp() string {
	return BoxStyle.Render(
		HeaderStyle.Render("Available Commands:") + "\n" +
			"/who - Show all users in the room\n" +
			"/me <action> - Perform an action\n" +
			"/help - Show this help message\n" +
			"/quit - Leave the chat",
	)
}

// FormatUserList formats the user list
func FormatUserList(roomName string, users []string, maxUsers int) string {
	roomName = SanitizeTerminalText(roomName)
	content := HeaderStyle.Render("Users in "+roomName+" ("+lipgloss.NewStyle().Foreground(accent).Render(fmt.Sprintf("%d/%d", len(users), maxUsers))+"):") + "\n"

	for _, user := range users {
		user = SanitizeTerminalText(user)
		userColor := GetUserColor(user)
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(userColor)).Bold(true)
		content += "- " + style.Render(user) + "\n"
	}

	return BoxStyle.Render(content)
}

// FormatWelcomeMessage formats the welcome message
func FormatWelcomeMessage(roomName, nickname string) string {
	roomName = SanitizeTerminalText(roomName)
	nickname = SanitizeTerminalText(nickname)
	return HeaderStyle.Render("Welcome to "+roomName+", "+nickname+"!") + "\n\n" +
		"Type a message and press Enter to send. Use /help to see available commands."
}
