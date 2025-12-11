package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bscott/ts-chat/internal/server"
	"github.com/spf13/pflag"
)

// Version info - set by ldflags at build time
var (
	Version = "dev"
	Commit  = "unknown"
)

// Default configuration values
const (
	defaultPort        = 2323
	defaultRoomName    = "Chat Room"
	defaultMaxUsers    = 10
	defaultHostname    = "chatroom"
	defaultHistorySize = 50
)

type config struct {
	Port            int
	RoomName        string
	MaxUsers        int
	EnableTailscale bool
	HostName        string
	EnableHistory   bool
	HistorySize     int
}

func main() {
	// Parse command-line flags
	cfg, showVersion := parseFlags()

	// Handle --version flag
	if showVersion {
		fmt.Printf("Chat Tails %s (commit: %s)\n", Version, Commit)
		os.Exit(0)
	}

	// Setup logger
	log.SetPrefix("[chat-tails] ")

	log.Printf("Chat Tails %s (commit: %s)", Version, Commit)

	if cfg.EnableTailscale {
		log.Printf("Starting with hostname: %s, port: %d", cfg.HostName, cfg.Port)
		
		// Check for auth key
		if os.Getenv("TS_AUTHKEY") == "" {
			log.Println("Warning: TS_AUTHKEY environment variable not set. Tailscale mode may not work properly.")
			log.Println("Set TS_AUTHKEY=tskey-... to authenticate with Tailscale")
		}
	} else {
		log.Printf("Starting Chat Tails on port: %d", cfg.Port)
	}

	// Create and start the chat server
	chatServer, err := server.NewServer(server.Config{
		Port:            cfg.Port,
		RoomName:        cfg.RoomName,
		MaxUsers:        cfg.MaxUsers,
		EnableTailscale: cfg.EnableTailscale,
		HostName:        cfg.HostName,
		EnableHistory:   cfg.EnableHistory,
		HistorySize:     cfg.HistorySize,
	})
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Start the server
	go func() {
		if err := chatServer.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	if cfg.EnableTailscale {
		log.Printf("Chat server started. Users can connect via: telnet %s.ts.net %d", cfg.HostName, cfg.Port)
	} else {
		log.Printf("Chat server started. Users can connect via: telnet localhost %d", cfg.Port)
	}
	
	log.Print("Press Ctrl+C to stop the server")

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Print("Shutting down server...")
	if err := chatServer.Stop(); err != nil {
		log.Printf("Error shutting down server: %v", err)
	}
	os.Exit(0)
}

func parseFlags() (config, bool) {
	var cfg config
	var showVersion bool

	// Define command-line flags
	pflag.IntVarP(&cfg.Port, "port", "p", defaultPort, "TCP port to listen on")
	pflag.StringVarP(&cfg.RoomName, "room-name", "r", defaultRoomName, "Chat room name")
	pflag.IntVarP(&cfg.MaxUsers, "max-users", "m", defaultMaxUsers, "Maximum allowed users")
	pflag.BoolVarP(&cfg.EnableTailscale, "tailscale", "t", false, "Enable Tailscale mode")
	pflag.StringVarP(&cfg.HostName, "hostname", "H", defaultHostname, "Tailscale hostname (only used if --tailscale is enabled)")
	pflag.BoolVar(&cfg.EnableHistory, "history", false, "Enable message history for new users")
	pflag.IntVar(&cfg.HistorySize, "history-size", defaultHistorySize, "Number of messages to keep in history")
	pflag.BoolVarP(&showVersion, "version", "v", false, "Show version information")

	// Display help message
	pflag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		pflag.PrintDefaults()
	}

	pflag.Parse()
	return cfg, showVersion
}