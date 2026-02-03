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
	MaxMessageLength = 1000                // Maximum message length in characters
	MessageRateLimit = 5                   // Maximum messages per second
	RateLimitWindow  = 5 * time.Second     // Time window for rate limiting
	MaxNicknameLen   = 20                  // Maximum nickname length
	MinNicknameLen   = 2                   // Minimum nickname length
)

// ANSI escape codes for terminal control
const (
	cursorUp        = "\033[1A"  // Move cursor up one line
	clearLine       = "\033[2K"  // Clear entire line
	cursorToStart   = "\033[0G"  // Move cursor to start of line
	inputPrompt     = "> "       // Input prompt indicator
)

// validateNickname checks if a nickname is valid
func validateNickname(nickname string) error {
	if nickname == "" {
		return fmt.Errorf("Nickname cannot be empty. Please try again.")
	}

	if len(nickname) < MinNicknameLen {
		return fmt.Errorf("Nickname must be at least %d characters.", MinNicknameLen)
	}

	if len(nickname) > MaxNicknameLen {
		return fmt.Errorf("Nickname must be at most %d characters.", MaxNicknameLen)
	}

	if strings.ToLower(nickname) == "system" {
		return fmt.Errorf("Nickname 'System' is reserved. Please choose another nickname.")
	}

	// Check for valid characters (alphanumeric, underscore, hyphen)
	for _, r := range nickname {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return fmt.Errorf("Nickname can only contain letters, numbers, underscores, and hyphens.")
		}
	}

	return nil
}

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
		// Leave the room since we encountered an error (this also removes the client/reservation)
		room.Leave(client)
		// Close the connection
		conn.Close()
		return nil, fmt.Errorf("welcome message failed: %w", err)
	}

	// Send message history to the new client
	client.sendHistory()

	return client, nil
}

// requestNickname asks the user for a nickname
func (c *Client) requestNickname() error {
	// Send welcome message
	var welcomeTitle string
	if c.room.PlainText {
		welcomeTitle = ui.FormatTitlePlain("Welcome to Chat Tails")
	} else {
		welcomeTitle = ui.FormatTitle("Welcome to Chat Tails")
	}
	if err := c.write(welcomeTitle + "\r\n\r\n"); err != nil {
		return fmt.Errorf("failed to write welcome message: %w", err)
	}
	
	// Ask for nickname
	for {
		if err := c.write("Please enter your nickname: "); err != nil {
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
		if err := validateNickname(nickname); err != nil {
			if writeErr := c.write(err.Error() + "\r\n"); writeErr != nil {
				return fmt.Errorf("failed to write error message: %w", writeErr)
			}
			continue
		}
		
		// Atomically reserve the nickname to prevent race conditions
		if !c.room.ReserveNickname(nickname) {
			errMsg := fmt.Sprintf("Nickname '%s' is already taken. Please choose another nickname.\r\n", nickname)
			if err := c.write(errMsg); err != nil {
				return fmt.Errorf("failed to write error message: %w", err)
			}
			continue
		}

		// Set nickname - reservation successful
		c.Nickname = nickname
		break
	}
	
	return nil
}

// sendWelcomeMessage sends a welcome message to the client
func (c *Client) sendWelcomeMessage() error {
	banner := `
╔════════════════════════════════════════════════════════════╗
║                                                            ║
║      _____ _           _     _____     _ _                 ║
║     / ____| |         | |   |_   _|   (_) |                ║
║    | |    | |__   __ _| |_    | | __ _ _| |___             ║
║    | |    | '_ \ / _' | __|   | |/ _' | | / __|            ║
║    | |____| | | | (_| | |_    | | (_| | | \__ \            ║
║     \_____|_| |_|\__,_|\__|   |_|\__,_|_|_|___/            ║
║                                                            ║
╚════════════════════════════════════════════════════════════╝
`
	var coloredBanner, welcomeMsg string

	if c.room.PlainText {
		// In plain text mode, skip the banner styling
		coloredBanner = banner
		welcomeMsg = ui.FormatWelcomeMessagePlain(c.room.Name, c.Nickname)
	} else {
		coloredBanner = ui.SystemStyle.Render(banner)
		welcomeMsg = ui.FormatWelcomeMessage(c.room.Name, c.Nickname)
	}

	if err := c.write(coloredBanner + "\r\n"); err != nil {
		return fmt.Errorf("failed to write banner: %w", err)
	}

	if err := c.write(welcomeMsg + "\r\n\r\n"); err != nil {
		return fmt.Errorf("failed to write welcome message: %w", err)
	}

	return nil
}

// sendHistory sends the message history to the client
func (c *Client) sendHistory() {
	history := c.room.GetHistory()
	if len(history) == 0 {
		return
	}

	var headerMsg, footerMsg string
	if c.room.PlainText {
		headerMsg = ui.FormatSystemMessagePlain("--- Recent messages ---")
		footerMsg = ui.FormatSystemMessagePlain("--- End of history ---")
	} else {
		headerMsg = ui.FormatSystemMessage("--- Recent messages ---")
		footerMsg = ui.FormatSystemMessage("--- End of history ---")
	}

	c.write(headerMsg + "\r\n")

	for _, msg := range history {
		c.sendMessage(msg)
	}

	c.write(footerMsg + "\r\n\r\n")
}

// Handle handles client interactions
func (c *Client) Handle(ctx context.Context) {
	// Cleanup when done
	defer func() {
		c.room.Leave(c)
		c.close()
	}()

	// Start a goroutine to close connection when context is cancelled
	go func() {
		<-ctx.Done()
		c.close()
	}()

	// Show initial prompt
	c.showPrompt()

	// Handle client messages
	for {
		// Set read deadline to allow periodic context checking
		if conn, ok := c.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		}

		line, err := c.reader.ReadString('\n')
		if err != nil {
			// Check if context was cancelled
			if ctx.Err() != nil {
				return
			}

			// Check for timeout - just continue to check context
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}

			if err == io.EOF {
				// Client disconnected normally
				return
			}

			log.Printf("Error reading from client %s: %v", c.Nickname, err)
			return
		}

		// Process the message
		message := strings.TrimSpace(line)

		// Clear the input line (move up and clear the echoed input)
		c.clearInputLine()

		// Skip empty messages
		if message == "" {
			c.showPrompt()
			continue
		}

		// Validate message length
		if err := c.validateMessageLength(message); err != nil {
			c.sendSystemMessage(fmt.Sprintf("Error: %v", err))
			c.showPrompt()
			continue
		}

		// Check rate limiting (except for /quit command)
		if !strings.HasPrefix(message, "/quit") {
			if err := c.checkRateLimit(); err != nil {
				c.sendSystemMessage(fmt.Sprintf("Error: %v", err))
				c.showPrompt()
				continue
			}
		}

		// Handle command or regular message
		if strings.HasPrefix(message, "/") {
			if err := c.handleCommand(message); err != nil {
				// Error already sent to client in handleCommand
			}
		} else {
			// Send message to room
			c.room.Broadcast(Message{
				From:      c.Nickname,
				Content:   message,
				Timestamp: time.Now(),
			})
		}

		// Show prompt for next input
		c.showPrompt()
	}
}

// clearInputLine clears the echoed input line
func (c *Client) clearInputLine() {
	// Skip cursor manipulation in plain text mode
	if !c.room.PlainText {
		c.write(cursorUp + clearLine + cursorToStart)
	}
}

// showPrompt displays the input prompt
func (c *Client) showPrompt() {
	c.write(inputPrompt)
}

// close closes the client connection safely
func (c *Client) close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
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
		c.close()
		return nil
		
	default:
		c.sendSystemMessage(fmt.Sprintf("Unknown command: %s", command))
		return fmt.Errorf("unknown command: %s", command)
	}
	
	return nil
}

// showUserList shows the list of users in the room
func (c *Client) showUserList() error {
	users := c.room.GetUserList()
	var msg string
	if c.room.PlainText {
		msg = ui.FormatUserListPlain(c.room.Name, users, c.room.MaxUsers)
	} else {
		msg = ui.FormatUserList(c.room.Name, users, c.room.MaxUsers)
	}
	return c.write(msg + "\r\n")
}

// showHelp shows the help message
func (c *Client) showHelp() error {
	var helpMsg string
	if c.room.PlainText {
		helpMsg = ui.FormatHelpPlain()
	} else {
		helpMsg = ui.FormatHelp()
	}
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

	if c.room.PlainText {
		// Use plain text formatters
		if msg.IsSystem {
			formatted = ui.FormatSystemMessagePlain(msg.Content) + "\r\n"
		} else if msg.IsAction {
			formatted = ui.FormatActionMessagePlain(msg.From, msg.Content) + "\r\n"
		} else {
			formatted = ui.FormatUserMessagePlain(msg.From, msg.Content, timeStr) + "\r\n"
		}
	} else {
		// Use ANSI formatters
		if msg.IsSystem {
			formatted = ui.FormatSystemMessage(msg.Content) + "\r\n"
		} else if msg.IsAction {
			formatted = ui.FormatActionMessage(msg.From, msg.Content) + "\r\n"
		} else {
			formatted = ui.FormatUserMessage(msg.From, msg.Content, timeStr) + "\r\n"
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if connection is still valid
	if c.conn == nil {
		return
	}

	if _, err := c.writer.WriteString(formatted); err != nil {
		log.Printf("Error writing message to %s: %v", c.Nickname, err)
		return
	}

	if err := c.writer.Flush(); err != nil {
		log.Printf("Error flushing message to %s: %v", c.Nickname, err)
		return
	}
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