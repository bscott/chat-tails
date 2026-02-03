package server

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