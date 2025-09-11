package chat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/bscott/ts-chat/internal/ui"
)

// Constants for rate limiting and validation
const (
	MaxMessageLength = 1000     // Maximum message length in characters
	MessageRateLimit = 5        // Maximum messages per second
	RateLimitWindow  = 5 * time.Second // Time window for rate limiting
)

// Client represents a chat client
type Client struct {
	Nickname          string
	conn              net.Conn
	reader            *bufio.Reader
	writer            *bufio.Writer
	room              *Room
	mu                sync.Mutex // Mutex to protect concurrent writes
	fullRoomRejection bool       // Flag indicating client was rejected due to room being full
	messageTimestamps []time.Time // Timestamps of recent messages for rate limiting
	rateLimitMu       sync.Mutex // Mutex for rate limiting data
}

// NewClient creates a new chat client
func NewClient(conn net.Conn, room *Room) (*Client, error) {
	client := &Client{
		conn:              conn,
		reader:            bufio.NewReader(conn),
		writer:            bufio.NewWriter(conn),
		room:              room,
		fullRoomRejection: false,
		messageTimestamps: make([]time.Time, 0, MessageRateLimit*2),
	}
	
	// Ask for nickname
	if err := client.requestNickname(); err != nil {
		// Ensure connection is closed on error
		conn.Close()
		return nil, fmt.Errorf("nickname request failed: %w", err)
	}
	
	// Join the room
	room.Join(client)
	
	// Check if client was rejected due to room being full
	if client.fullRoomRejection {
		// Close the connection since the room is full
		conn.Close()
		return nil, fmt.Errorf("room is full")
	}
	
	// Send welcome message
	if err := client.sendWelcomeMessage(); err != nil {
		// Leave the room since we encountered an error
		room.Leave(client)
		// Close the connection
		conn.Close()
		return nil, fmt.Errorf("welcome message failed: %w", err)
	}
	
	return client, nil
}

// requestNickname asks the user for a nickname
func (c *Client) requestNickname() error {
	// Send welcome message
	if err := c.write(ui.FormatTitle("Welcome to Tailscale Terminal Chat") + "\r\n\r\n"); err != nil {
		return fmt.Errorf("failed to write welcome message: %w", err)
	}
	
	// Ask for nickname
	for {
		if err := c.write(ui.InputStyle.Render("Please enter your nickname: ")); err != nil {
			return fmt.Errorf("failed to write nickname prompt: %w", err)
		}
		
		// Read nickname
		nickname, err := c.reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read nickname: %w", err)
		}
		
		// Trim whitespace
		nickname = strings.TrimSpace(nickname)
		
		// Validate nickname
		if nickname == "" {
			if err := c.write("Nickname cannot be empty. Please try again.\r\n"); err != nil {
				return fmt.Errorf("failed to write error message: %w", err)
			}
			continue
		}
		
		if strings.ToLower(nickname) == "system" {
			if err := c.write("Nickname 'System' is reserved. Please choose another nickname.\r\n"); err != nil {
				return fmt.Errorf("failed to write error message: %w", err)
			}
			continue
		}
		
		if !c.room.IsNicknameAvailable(nickname) {
			errMsg := fmt.Sprintf("Nickname '%s' is already taken. Please choose another nickname.\r\n", nickname)
			if err := c.write(errMsg); err != nil {
				return fmt.Errorf("failed to write error message: %w", err)
			}
			continue
		}
		
		// Set nickname
		c.Nickname = nickname
		break
	}
	
	return nil
}

// sendWelcomeMessage sends a welcome message to the client
func (c *Client) sendWelcomeMessage() error {
	banner := `
╔═══════════════════════════════════════════════════════════════════════╗
║           _____                    _             _   _____             ║
║          |_   _|__ _ __ _ __ ___ (_)_ __   __ _| | |  __ \            ║
║            | |/ _ \ '__| '_ ' _ \| | '_ \ / _' | | | |  | |           ║
║            | |  __/ |  | | | | | | | | | | (_| | | | |__| |           ║
║            |_|\___|_|  |_| |_| |_|_|_| |_|\__,_|_| |_____/            ║
║                                                                       ║
║                             CHAT ROOM                                 ║
╚═══════════════════════════════════════════════════════════════════════╝
`
	coloredBanner := ui.SystemStyle.Render(banner)
	welcomeMsg := ui.FormatWelcomeMessage(c.room.Name, c.Nickname)
	
	if err := c.write(coloredBanner + "\r\n"); err != nil {
		return fmt.Errorf("failed to write banner: %w", err)
	}
	
	if err := c.write(welcomeMsg + "\r\n\r\n"); err != nil {
		return fmt.Errorf("failed to write welcome message: %w", err)
	}
	
	if err := c.write("Type a message and press Enter to send. Type /help for commands.\r\n\r\n"); err != nil {
		return fmt.Errorf("failed to write help message: %w", err)
	}
	
	return nil
}

// Handle handles client interactions
func (c *Client) Handle(ctx context.Context) {
	// Cleanup when done
	defer func() {
		c.room.Leave(c)
	}()
	
	// Create a timeout reader
	readCh := make(chan readResult)
	readErrorCh := make(chan error)
	
	// Handle client messages
	for {
		select {
		case <-ctx.Done():
			return
			
		default:
			// Use a goroutine for reading to handle timeouts and cancelations
			go func() {
				line, err := c.reader.ReadString('\n')
				if err != nil {
					readErrorCh <- err
					return
				}
				readCh <- readResult{message: line}
			}()
			
			// Wait for either a message, error, or context cancellation
			select {
			case <-ctx.Done():
				return
				
			case err := <-readErrorCh:
				if err == io.EOF {
					// Client disconnected normally
					return
				}
				
				// Try to notify the client of the error
				log.Printf("Error reading from client %s: %v", c.Nickname, err)
				c.sendSystemMessage(fmt.Sprintf("Error reading message: %v", err))
				return
				
			case result := <-readCh:
				// Process the message
				message := strings.TrimSpace(result.message)
				
				// Skip empty messages
				if message == "" {
					continue
				}
				
				// Validate message length
				if err := c.validateMessageLength(message); err != nil {
					c.sendSystemMessage(fmt.Sprintf("Error: %v", err))
					continue
				}
				
				// Check rate limiting (except for /quit command)
				if !strings.HasPrefix(message, "/quit") {
					if err := c.checkRateLimit(); err != nil {
						c.sendSystemMessage(fmt.Sprintf("Error: %v", err))
						continue
					}
				}
				
				// Handle command or regular message
				if strings.HasPrefix(message, "/") {
					if err := c.handleCommand(message); err != nil {
						c.sendSystemMessage(fmt.Sprintf("Error: %v", err))
					}
				} else {
					// Send message to room
					c.room.Broadcast(Message{
						From:      c.Nickname,
						Content:   message,
						Timestamp: time.Now(),
					})
				}
			}
		}
	}
}

// readResult holds the result of a read operation
type readResult struct {
	message string
}

// validateMessageLength checks if a message is within the allowed length
func (c *Client) validateMessageLength(message string) error {
	if len(message) > MaxMessageLength {
		return fmt.Errorf("message too long (max %d characters)", MaxMessageLength)
	}
	return nil
}

// checkRateLimit checks if the client is sending messages too quickly
func (c *Client) checkRateLimit() error {
	now := time.Now()
	c.rateLimitMu.Lock()
	defer c.rateLimitMu.Unlock()
	
	// Add current timestamp
	c.messageTimestamps = append(c.messageTimestamps, now)
	
	// Remove timestamps outside the window
	cutoff := now.Add(-RateLimitWindow)
	newTimestamps := make([]time.Time, 0, len(c.messageTimestamps))
	
	for _, ts := range c.messageTimestamps {
		if ts.After(cutoff) {
			newTimestamps = append(newTimestamps, ts)
		}
	}
	
	c.messageTimestamps = newTimestamps
	
	// Check if we have too many messages in the window
	if len(c.messageTimestamps) > MessageRateLimit {
		waitTime := c.messageTimestamps[0].Add(RateLimitWindow).Sub(now)
		return fmt.Errorf("rate limit exceeded (max %d messages per %s). Try again in %.1f seconds", 
			MessageRateLimit, RateLimitWindow, waitTime.Seconds())
	}
	
	return nil
}

// handleCommand handles a command from the client
func (c *Client) handleCommand(cmd string) error {
	parts := strings.SplitN(cmd, " ", 2)
	command := strings.ToLower(parts[0])
	
	switch command {
	case "/who":
		return c.showUserList()
		
	case "/me":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			c.sendSystemMessage("Usage: /me <action>")
			return fmt.Errorf("invalid /me command usage")
		}
		action := parts[1]
		c.room.Broadcast(Message{
			From:      c.Nickname,
			Content:   action,
			Timestamp: time.Now(),
			IsAction:  true,
		})
		
	case "/help":
		return c.showHelp()
		
	case "/quit":
		c.sendSystemMessage("Goodbye!")
		// We don't return an error here since this is expected behavior
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("error closing connection: %w", err)
		}
		
	default:
		c.sendSystemMessage(fmt.Sprintf("Unknown command: %s", command))
		return fmt.Errorf("unknown command: %s", command)
	}
	
	return nil
}

// showUserList shows the list of users in the room
func (c *Client) showUserList() error {
	users := c.room.GetUserList()
	msg := ui.FormatUserList(c.room.Name, users, c.room.MaxUsers)
	return c.write(msg + "\r\n")
}

// showHelp shows the help message
func (c *Client) showHelp() error {
	helpMsg := ui.FormatHelp()
	return c.write(helpMsg + "\r\n")
}

// sendSystemMessage sends a system message to the client
func (c *Client) sendSystemMessage(message string) {
	msg := Message{
		From:      "System",
		Content:   message,
		Timestamp: time.Now(),
		IsSystem:  true,
	}
	
	c.sendMessage(msg)
}

// sendMessage sends a message to the client
func (c *Client) sendMessage(msg Message) {
	var formatted string
	timeStr := msg.Timestamp.Format("15:04:05")
	
	
	if msg.IsSystem {
		formatted = ui.FormatSystemMessage(msg.Content) + "\r\n"
	} else if msg.IsAction {
		formatted = ui.FormatActionMessage(msg.From, msg.Content) + "\r\n"
	} else if msg.From == c.Nickname {
		formatted = ui.FormatSelfMessage(msg.Content, timeStr) + "\r\n"
	} else {
		formatted = ui.FormatUserMessage(msg.From, msg.Content, timeStr) + "\r\n"
	}
	
	// Use a safer approach to write to client
	// Create a channel to receive any errors from the goroutine
	errCh := make(chan error, 1)
	
	// Ensure the write happens without blocking
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic in sendMessage for client %s: %v", c.Nickname, r)
				errCh <- fmt.Errorf("panic in sendMessage: %v", r)
			}
			close(errCh)
		}()
		
		c.mu.Lock()
		defer c.mu.Unlock()
		
		// Check if connection is still valid
		if c.conn == nil {
			errCh <- fmt.Errorf("connection closed")
			return
		}
		
		if _, err := c.writer.WriteString(formatted); err != nil {
			errCh <- fmt.Errorf("error writing message: %w", err)
			return
		}
		
		if err := c.writer.Flush(); err != nil {
			errCh <- fmt.Errorf("error flushing message: %w", err)
			return
		}
	}()
	
	// Log any errors (non-blocking)
	go func() {
		for err := range errCh {
			log.Printf("Error sending message to %s: %v", c.Nickname, err)
		}
	}()
}

// write writes a message to the client
func (c *Client) write(message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Check if connection is still valid
	if c.conn == nil {
		return fmt.Errorf("connection closed")
	}
	
	if _, err := c.writer.WriteString(message); err != nil {
		return fmt.Errorf("error writing message: %w", err)
	}
	
	if err := c.writer.Flush(); err != nil {
		return fmt.Errorf("error flushing message: %w", err)
	}
	
	return nil
}