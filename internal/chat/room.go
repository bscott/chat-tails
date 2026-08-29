package chat

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Message represents a chat message
type Message struct {
	From      string
	Content   string
	Timestamp time.Time
	IsSystem  bool
	IsAction  bool
}

var (
	ErrRoomFull   = errors.New("room is full")
	ErrRoomClosed = errors.New("room is closed")
)

type joinRequest struct {
	client *Client
	result chan error
}

// Room represents a chat room
type Room struct {
	Name          string
	MaxUsers      int
	clients       map[string]*Client
	broadcast     chan Message
	join          chan joinRequest
	leave         chan *Client
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	stopOnce      sync.Once
	enableHistory bool
	historySize   int
	history       []Message
	historyMu     sync.RWMutex
	PlainText     bool
}

// NewRoom creates a new chat room
func NewRoom(name string, maxUsers int, enableHistory bool, historySize int, plainText bool) *Room {
	ctx, cancel := context.WithCancel(context.Background())
	room := &Room{
		Name:          name,
		MaxUsers:      maxUsers,
		clients:       make(map[string]*Client),
		broadcast:     make(chan Message),
		join:          make(chan joinRequest),
		leave:         make(chan *Client),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
		enableHistory: enableHistory,
		historySize:   historySize,
		history:       make([]Message, 0, historySize),
		PlainText:     plainText,
	}

	go room.run()
	return room
}

// run handles room events
func (r *Room) run() {
	defer close(r.done)
	for {
		select {
		case <-r.ctx.Done():
			return
		case request := <-r.join:
			request.result <- r.addClient(request.client)
		case client := <-r.leave:
			r.removeClient(client)
		case msg := <-r.broadcast:
			r.broadcastMessage(msg)
		}
	}
}

// addClient adds a client to the room.
func (r *Room) addClient(c *Client) error {
	r.mu.Lock()

	activeClients := 0
	for _, client := range r.clients {
		if client != nil {
			activeClients++
		}
	}

	if activeClients >= r.MaxUsers {
		if client, exists := r.clients[c.Nickname]; exists && client == nil {
			delete(r.clients, c.Nickname)
		}
		r.mu.Unlock()
		return ErrRoomFull
	}

	// Add client to the room (replaces nil reservation with actual client)
	r.clients[c.Nickname] = c
	r.mu.Unlock()

	// Notify everyone that a new user has joined (outside of lock to avoid deadlock)
	systemMsg := Message{
		From:      "System",
		Content:   fmt.Sprintf("%s has joined the room", c.Nickname),
		Timestamp: time.Now(),
		IsSystem:  true,
	}
	r.broadcastMessage(systemMsg)
	return nil
}

// removeClient removes a client from the room
func (r *Room) removeClient(c *Client) {
	r.mu.Lock()
	client, exists := r.clients[c.Nickname]
	if exists && client == c {
		delete(r.clients, c.Nickname)
	} else {
		exists = false
	}
	r.mu.Unlock()

	if exists {
		// Notify everyone that a user has left (outside of lock to avoid deadlock)
		systemMsg := Message{
			From:      "System",
			Content:   fmt.Sprintf("%s has left the room", c.Nickname),
			Timestamp: time.Now(),
			IsSystem:  true,
		}
		r.broadcastMessage(systemMsg)
	}
}

// broadcastMessage sends a message to all clients
func (r *Room) broadcastMessage(msg Message) {
	// Store in history if enabled (for non-system messages or join/leave messages)
	if r.enableHistory {
		r.addToHistory(msg)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, client := range r.clients {
		if client != nil {
			go client.Send(msg) // Use goroutine to avoid blocking
		}
	}
}

// addToHistory adds a message to the history buffer
func (r *Room) addToHistory(msg Message) {
	r.historyMu.Lock()
	defer r.historyMu.Unlock()

	r.history = append(r.history, msg)

	// Trim history if it exceeds the max size
	if len(r.history) > r.historySize {
		r.history = r.history[len(r.history)-r.historySize:]
	}
}

// GetHistory returns the message history
func (r *Room) GetHistory() []Message {
	r.historyMu.RLock()
	defer r.historyMu.RUnlock()

	// Return a copy to avoid race conditions
	history := make([]Message, len(r.history))
	copy(history, r.history)
	return history
}

// Join adds a client to the room and waits for the admission decision.
func (r *Room) Join(client *Client) error {
	request := joinRequest{
		client: client,
		result: make(chan error, 1),
	}

	select {
	case r.join <- request:
		return <-request.result
	case <-r.ctx.Done():
		r.ReleaseNickname(client.Nickname)
		return ErrRoomClosed
	}
}

// Leave removes a client from the room
func (r *Room) Leave(client *Client) {
	select {
	case r.leave <- client:
	case <-r.ctx.Done():
		// Room is shutting down, don't block
	}
}

// Broadcast sends a message to all clients
func (r *Room) Broadcast(msg Message) {
	select {
	case r.broadcast <- msg:
	case <-r.ctx.Done():
		// Room is shutting down, don't block
	}
}

// GetUserList returns a list of all users in the room
func (r *Room) GetUserList() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	users := make([]string, 0, len(r.clients))
	for nickname := range r.clients {
		users = append(users, nickname)
	}

	return users
}

// IsNicknameAvailable checks if a nickname is available
func (r *Room) IsNicknameAvailable(nickname string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.clients[nickname]
	return !exists
}

// ReserveNickname atomically checks and reserves a nickname, returning true if successful
func (r *Room) ReserveNickname(nickname string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.clients[nickname]; exists {
		return false
	}

	// Reserve with a nil client temporarily - will be replaced by actual client on Join
	r.clients[nickname] = nil
	return true
}

// ReleaseNickname releases a reserved nickname if the client is nil (reservation only)
func (r *Room) ReleaseNickname(nickname string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if client, exists := r.clients[nickname]; exists && client == nil {
		delete(r.clients, nickname)
	}
}

// Stop gracefully shuts down the room.
func (r *Room) Stop() error {
	r.stopOnce.Do(r.cancel)
	<-r.done
	return nil
}
