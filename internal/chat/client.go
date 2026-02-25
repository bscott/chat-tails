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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bscott/ts-chat/internal/ui"
)

// Constants for rate limiting and validation
const (
	MaxMessageLength = 1000            // Maximum message length in characters
	MessageRateLimit = 5               // Maximum messages per window
	RateLimitWindow  = 5 * time.Second // Time window for rate limiting
	MaxNicknameLen   = 20              // Maximum nickname length
	MinNicknameLen   = 2               // Minimum nickname length
)

// ANSI escape codes for terminal control (plain-text mode only)
const (
	cursorUp      = "\033[1A"
	clearLine     = "\033[2K"
	cursorToStart = "\033[0G"
	inputPrompt   = "> "
)

// Telnet negotiation bytes for character-at-a-time mode
var telnetNegotiation = []byte{
	255, 251, 1, // IAC WILL ECHO
	255, 251, 3, // IAC WILL SUPPRESS-GO-AHEAD
	255, 253, 3, // IAC DO SUPPRESS-GO-AHEAD
}

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
	mu                sync.Mutex
	fullRoomRejection bool
	messageTimestamps []time.Time
	rateLimitMu       sync.Mutex
	program           *tea.Program // set in TUI mode, nil in plain-text mode
}

// NewTUIClient creates a client for TUI (bubbletea) mode.
// Nickname negotiation happens inside the bubbletea model.
func NewTUIClient(conn net.Conn, room *Room) *Client {
	return &Client{
		conn:              conn,
		room:              room,
		messageTimestamps: make([]time.Time, 0, MessageRateLimit*2),
	}
}

// Send delivers a message to this client. In TUI mode it uses program.Send(),
// in plain-text mode it writes directly to the connection.
func (c *Client) Send(msg Message) {
	if c.program != nil {
		c.program.Send(ChatMsg{Message: msg})
		return
	}
	c.sendMessage(msg)
}

// --- TUI mode (bubbletea) ---

// RunTUI starts the bubbletea program for this client over the TCP connection.
// It blocks until the user quits.
func (c *Client) RunTUI(ctx context.Context) {
	// Send telnet negotiation to enable character-at-a-time mode
	c.conn.Write(telnetNegotiation)

	model := NewChatModel(c)

	p := tea.NewProgram(
		model,
		tea.WithInput(c.conn),
		tea.WithOutput(c.conn),
		tea.WithAltScreen(),
	)
	c.program = p

	// Close connection when context is cancelled
	go func() {
		<-ctx.Done()
		p.Quit()
	}()

	if _, err := p.Run(); err != nil {
		log.Printf("TUI error for %s: %v", c.Nickname, err)
	}
}

// --- Plain-text mode (legacy telnet) ---

// NewPlainTextClient creates a client for plain-text mode with nickname negotiation.
func NewPlainTextClient(conn net.Conn, room *Room) (*Client, error) {
	client := &Client{
		conn:              conn,
		reader:            bufio.NewReader(conn),
		writer:            bufio.NewWriter(conn),
		room:              room,
		fullRoomRejection: false,
		messageTimestamps: make([]time.Time, 0, MessageRateLimit*2),
	}

	if err := client.requestNickname(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("nickname request failed: %w", err)
	}

	room.Join(client)

	if client.fullRoomRejection {
		conn.Close()
		return nil, fmt.Errorf("room is full")
	}

	if err := client.sendWelcomeMessage(); err != nil {
		room.Leave(client)
		conn.Close()
		return nil, fmt.Errorf("welcome message failed: %w", err)
	}

	client.sendHistory()

	return client, nil
}

func (c *Client) requestNickname() error {
	var welcomeTitle string
	if c.room.PlainText {
		welcomeTitle = ui.FormatTitlePlain("Welcome to Chat Tails")
	} else {
		welcomeTitle = ui.FormatTitle("Welcome to Chat Tails")
	}
	if err := c.write(welcomeTitle + "\r\n\r\n"); err != nil {
		return fmt.Errorf("failed to write welcome message: %w", err)
	}

	for {
		if err := c.write("Please enter your nickname: "); err != nil {
			return fmt.Errorf("failed to write nickname prompt: %w", err)
		}

		nickname, err := c.reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read nickname: %w", err)
		}

		nickname = strings.TrimSpace(nickname)

		if err := validateNickname(nickname); err != nil {
			if writeErr := c.write(err.Error() + "\r\n"); writeErr != nil {
				return fmt.Errorf("failed to write error message: %w", writeErr)
			}
			continue
		}

		if !c.room.ReserveNickname(nickname) {
			errMsg := fmt.Sprintf("Nickname '%s' is already taken. Please choose another nickname.\r\n", nickname)
			if err := c.write(errMsg); err != nil {
				return fmt.Errorf("failed to write error message: %w", err)
			}
			continue
		}

		c.Nickname = nickname
		break
	}

	return nil
}

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

// Handle handles client interactions in plain-text mode.
func (c *Client) Handle(ctx context.Context) {
	defer func() {
		c.room.Leave(c)
		c.close()
	}()

	go func() {
		<-ctx.Done()
		c.close()
	}()

	c.showPrompt()

	for {
		if conn, ok := c.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		}

		line, err := c.reader.ReadString('\n')
		if err != nil {
			if ctx.Err() != nil {
				return
			}

			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}

			if err == io.EOF {
				return
			}

			log.Printf("Error reading from client %s: %v", c.Nickname, err)
			return
		}

		message := strings.TrimSpace(line)

		c.clearInputLine()

		if message == "" {
			c.showPrompt()
			continue
		}

		if err := c.validateMessageLength(message); err != nil {
			c.sendSystemMessage(fmt.Sprintf("Error: %v", err))
			c.showPrompt()
			continue
		}

		if !strings.HasPrefix(message, "/quit") {
			if err := c.checkRateLimit(); err != nil {
				c.sendSystemMessage(fmt.Sprintf("Error: %v", err))
				c.showPrompt()
				continue
			}
		}

		if strings.HasPrefix(message, "/") {
			c.handleCommand(message)
		} else {
			c.room.Broadcast(Message{
				From:      c.Nickname,
				Content:   message,
				Timestamp: time.Now(),
			})
		}

		c.showPrompt()
	}
}

func (c *Client) clearInputLine() {
	if !c.room.PlainText {
		c.write(cursorUp + clearLine + cursorToStart)
	}
}

func (c *Client) showPrompt() {
	c.write(inputPrompt)
}

func (c *Client) close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func (c *Client) validateMessageLength(message string) error {
	if len(message) > MaxMessageLength {
		return fmt.Errorf("message too long (max %d characters)", MaxMessageLength)
	}
	return nil
}

func (c *Client) checkRateLimit() error {
	now := time.Now()
	c.rateLimitMu.Lock()
	defer c.rateLimitMu.Unlock()

	c.messageTimestamps = append(c.messageTimestamps, now)

	cutoff := now.Add(-RateLimitWindow)
	newTimestamps := make([]time.Time, 0, len(c.messageTimestamps))

	for _, ts := range c.messageTimestamps {
		if ts.After(cutoff) {
			newTimestamps = append(newTimestamps, ts)
		}
	}

	c.messageTimestamps = newTimestamps

	if len(c.messageTimestamps) > MessageRateLimit {
		waitTime := c.messageTimestamps[0].Add(RateLimitWindow).Sub(now)
		return fmt.Errorf("rate limit exceeded (max %d messages per %s). Try again in %.1f seconds",
			MessageRateLimit, RateLimitWindow, waitTime.Seconds())
	}

	return nil
}

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

func (c *Client) showHelp() error {
	var helpMsg string
	if c.room.PlainText {
		helpMsg = ui.FormatHelpPlain()
	} else {
		helpMsg = ui.FormatHelp()
	}
	return c.write(helpMsg + "\r\n")
}

func (c *Client) sendSystemMessage(message string) {
	msg := Message{
		From:      "System",
		Content:   message,
		Timestamp: time.Now(),
		IsSystem:  true,
	}

	c.sendMessage(msg)
}

func (c *Client) sendMessage(msg Message) {
	var formatted string
	timeStr := msg.Timestamp.Format("15:04:05")

	if c.room.PlainText {
		if msg.IsSystem {
			formatted = ui.FormatSystemMessagePlain(msg.Content) + "\r\n"
		} else if msg.IsAction {
			formatted = ui.FormatActionMessagePlain(msg.From, msg.Content) + "\r\n"
		} else {
			formatted = ui.FormatUserMessagePlain(msg.From, msg.Content, timeStr) + "\r\n"
		}
	} else {
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

func (c *Client) write(message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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
