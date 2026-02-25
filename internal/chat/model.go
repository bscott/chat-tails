package chat

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bscott/ts-chat/internal/ui"
)

type modelState int

const (
	stateNickname modelState = iota
	stateChat
)

// ChatModel is the bubbletea model for a single client connection.
type ChatModel struct {
	state     modelState
	viewport  viewport.Model
	textInput textinput.Model
	messages  []Message
	client    *Client
	width     int
	height    int
	ready     bool
	errMsg    string
	quitting  bool
}

// NewChatModel creates a model in the nickname-entry state.
func NewChatModel(client *Client) ChatModel {
	ti := textinput.New()
	ti.Placeholder = "Enter nickname..."
	ti.CharLimit = MaxNicknameLen
	ti.Width = 40
	ti.Focus()

	return ChatModel{
		state:     stateNickname,
		textInput: ti,
		client:    client,
		width:     80,
		height:    24,
	}
}

func (m ChatModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.state == stateChat {
			m.resizeViewport()
		}
		return m, nil

	case tea.KeyMsg:
		// Global keybindings
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit
		}

		switch m.state {
		case stateNickname:
			return m.updateNickname(msg)
		case stateChat:
			return m.updateChat(msg)
		}

	case ChatMsg:
		return m.handleChatMsg(msg)

	case JoinedMsg:
		return m.handleJoined()

	case RoomFullMsg:
		m.errMsg = "Room is full. Disconnecting..."
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m ChatModel) View() string {
	if m.quitting {
		if m.errMsg != "" {
			return ui.FormatSystemMessage(m.errMsg) + "\n"
		}
		return ui.FormatSystemMessage("Goodbye!") + "\n"
	}

	switch m.state {
	case stateNickname:
		return m.nicknameView()
	case stateChat:
		return m.chatView()
	}

	return ""
}

// --- Nickname state ---

func (m ChatModel) updateNickname(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		nickname := strings.TrimSpace(m.textInput.Value())

		if err := validateNickname(nickname); err != nil {
			m.errMsg = err.Error()
			m.textInput.Reset()
			return m, nil
		}

		if !m.client.room.ReserveNickname(nickname) {
			m.errMsg = fmt.Sprintf("Nickname '%s' is already taken.", nickname)
			m.textInput.Reset()
			return m, nil
		}

		m.client.Nickname = nickname
		m.errMsg = ""

		// Join room asynchronously via Cmd
		return m, m.joinRoomCmd()

	case tea.KeyEsc:
		m.quitting = true
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m ChatModel) joinRoomCmd() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		client.room.Join(client)
		if client.fullRoomRejection {
			return RoomFullMsg{}
		}
		return JoinedMsg{}
	}
}

func (m ChatModel) nicknameView() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4"))

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#383838"))

	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(titleStyle.Render("  Chat Tails"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("  Terminal chat over TCP & Tailscale"))
	b.WriteString("\n\n")

	b.WriteString("  " + m.textInput.View())
	b.WriteString("\n\n")

	if m.errMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F25D94"))
		b.WriteString("  " + errStyle.Render(m.errMsg))
		b.WriteString("\n\n")
	}

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#626262"))
	b.WriteString(helpStyle.Render("  enter: confirm • esc: quit"))
	b.WriteString("\n")

	return b.String()
}

// --- Chat state ---

func (m ChatModel) handleJoined() (tea.Model, tea.Cmd) {
	m.state = stateChat
	m.initViewport()
	m.errMsg = ""

	// Reconfigure text input for chat mode
	m.textInput.Placeholder = "Type a message..."
	m.textInput.CharLimit = MaxMessageLength
	m.textInput.Width = m.width - 4
	m.textInput.Reset()

	// Load message history
	history := m.client.room.GetHistory()
	for _, msg := range history {
		m.messages = append(m.messages, msg)
	}
	m.updateViewportContent()

	return m, nil
}

func (m ChatModel) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		message := strings.TrimSpace(m.textInput.Value())
		m.textInput.Reset()

		if message == "" {
			return m, nil
		}

		if len(message) > MaxMessageLength {
			m.appendSystemMessage(fmt.Sprintf("Message too long (max %d characters)", MaxMessageLength))
			return m, nil
		}

		// Check rate limit
		if err := m.client.checkRateLimit(); err != nil {
			m.appendSystemMessage(fmt.Sprintf("Error: %v", err))
			return m, nil
		}

		// Handle commands
		if strings.HasPrefix(message, "/") {
			return m.handleCommand(message)
		}

		// Broadcast regular message
		m.client.room.Broadcast(Message{
			From:      m.client.Nickname,
			Content:   message,
			Timestamp: time.Now(),
		})
		return m, nil

	case tea.KeyEsc:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyPgUp:
		m.viewport.HalfPageUp()
		return m, nil

	case tea.KeyPgDown:
		m.viewport.HalfPageDown()
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m *ChatModel) handleCommand(cmd string) (tea.Model, tea.Cmd) {
	parts := strings.SplitN(cmd, " ", 2)
	command := strings.ToLower(parts[0])

	switch command {
	case "/who":
		users := m.client.room.GetUserList()
		userList := fmt.Sprintf("Users in %s (%d/%d):", m.client.room.Name, len(users), m.client.room.MaxUsers)
		for _, user := range users {
			userList += "\n  - " + user
		}
		m.appendSystemMessage(userList)

	case "/me":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			m.appendSystemMessage("Usage: /me <action>")
			return m, nil
		}
		m.client.room.Broadcast(Message{
			From:      m.client.Nickname,
			Content:   parts[1],
			Timestamp: time.Now(),
			IsAction:  true,
		})

	case "/help":
		help := "Commands:\n" +
			"  /who    - Show online users\n" +
			"  /me     - Perform an action\n" +
			"  /help   - Show this help\n" +
			"  /quit   - Leave the chat"
		m.appendSystemMessage(help)

	case "/quit":
		m.quitting = true
		return m, tea.Quit

	default:
		m.appendSystemMessage(fmt.Sprintf("Unknown command: %s", command))
	}

	return m, nil
}

func (m ChatModel) handleChatMsg(msg ChatMsg) (tea.Model, tea.Cmd) {
	m.messages = append(m.messages, msg.Message)
	wasAtBottom := m.viewport.AtBottom()
	m.updateViewportContent()
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
	return m, nil
}

// --- Viewport helpers ---

func (m *ChatModel) initViewport() {
	headerHeight := 1 // status bar
	inputHeight := 3  // input area with border
	vpHeight := m.height - headerHeight - inputHeight - 1
	if vpHeight < 3 {
		vpHeight = 3
	}

	m.viewport = viewport.New(m.width, vpHeight)
	m.viewport.Style = lipgloss.NewStyle()
	m.ready = true
}

func (m *ChatModel) resizeViewport() {
	headerHeight := 1
	inputHeight := 3
	vpHeight := m.height - headerHeight - inputHeight - 1
	if vpHeight < 3 {
		vpHeight = 3
	}
	m.viewport.Width = m.width
	m.viewport.Height = vpHeight
	m.textInput.Width = m.width - 4
}

func (m *ChatModel) updateViewportContent() {
	var lines []string
	for _, msg := range m.messages {
		lines = append(lines, m.formatMessage(msg))
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m *ChatModel) formatMessage(msg Message) string {
	timeStr := msg.Timestamp.Format("15:04:05")

	if msg.IsSystem {
		return ui.FormatSystemMessage(msg.Content)
	}
	if msg.IsAction {
		return ui.FormatActionMessage(msg.From, msg.Content)
	}
	return ui.FormatUserMessage(msg.From, msg.Content, timeStr)
}

func (m *ChatModel) appendSystemMessage(content string) {
	msg := Message{
		From:      "System",
		Content:   content,
		Timestamp: time.Now(),
		IsSystem:  true,
	}
	m.messages = append(m.messages, msg)
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

// --- Chat view ---

func (m ChatModel) chatView() string {
	if !m.ready {
		return "Initializing...\n"
	}

	// Status bar
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	statusInfoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(lipgloss.Color("#4A2DB0")).
		Padding(0, 1)

	users := m.client.room.GetUserList()
	statusLeft := statusStyle.Render(m.client.room.Name)
	statusRight := statusInfoStyle.Render(fmt.Sprintf("%s | %d online", m.client.Nickname, len(users)))

	statusGap := m.width - lipgloss.Width(statusLeft) - lipgloss.Width(statusRight)
	if statusGap < 0 {
		statusGap = 0
	}
	statusBar := statusLeft +
		lipgloss.NewStyle().
			Background(lipgloss.Color("#4A2DB0")).
			Render(strings.Repeat(" ", statusGap)) +
		statusRight

	// Input area
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Width(m.width - 2)

	input := inputStyle.Render(m.textInput.View())

	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewport.View(),
		statusBar,
		input,
	)
}
