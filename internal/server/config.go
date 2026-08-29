package server

import (
	"errors"
	"strings"
)

// Config holds the server configuration
type Config struct {
	Port            int    // TCP port to listen on
	RoomName        string // Chat room name
	MaxUsers        int    // Maximum allowed users
	EnableTailscale bool   // Whether to enable Tailscale mode
	HostName        string // Tailscale hostname (only used if EnableTailscale is true)
	EnableHistory   bool   // Whether to enable message history for new users
	HistorySize     int    // Number of messages to keep in history
	PlainText       bool   // Whether to disable ANSI formatting (for Windows telnet compatibility)
}

func (c Config) validate() error {
	switch {
	case c.Port < 0 || c.Port > 65535:
		return errors.New("port must be between 0 and 65535")
	case strings.TrimSpace(c.RoomName) == "":
		return errors.New("room name must not be empty")
	case c.MaxUsers <= 0:
		return errors.New("max users must be greater than zero")
	case c.HistorySize < 0:
		return errors.New("history size must not be negative")
	case c.EnableHistory && c.HistorySize == 0:
		return errors.New("history size must be greater than zero when history is enabled")
	case c.EnableTailscale && strings.TrimSpace(c.HostName) == "":
		return errors.New("hostname must not be empty when Tailscale is enabled")
	default:
		return nil
	}
}
