package chat

import (
	"testing"
	"time"
)

func TestNewRoom(t *testing.T) {
	room := NewRoom("Test Room", 10)
	defer room.Stop()
	
	if room.Name != "Test Room" {
		t.Errorf("Expected room name 'Test Room', got %s", room.Name)
	}
	
	if room.MaxUsers != 10 {
		t.Errorf("Expected max users 10, got %d", room.MaxUsers)
	}
	
	if room.clients == nil {
		t.Error("Expected clients map to be initialized")
	}
	
	if room.broadcast == nil {
		t.Error("Expected broadcast channel to be initialized")
	}
	
	if room.join == nil {
		t.Error("Expected join channel to be initialized")
	}
	
	if room.leave == nil {
		t.Error("Expected leave channel to be initialized")
	}
}

func TestRoomStop(t *testing.T) {
	room := NewRoom("Test Room", 10)
	
	// Give room time to start
	time.Sleep(10 * time.Millisecond)
	
	// Stop the room
	err := room.Stop()
	if err != nil {
		t.Errorf("Failed to stop room: %v", err)
	}
	
	// Check that done channel is closed
	select {
	case <-room.done:
		// Successfully closed
	case <-time.After(100 * time.Millisecond):
		t.Error("Room done channel not closed after Stop()")
	}
}

func TestMessageStruct(t *testing.T) {
	now := time.Now()
	msg := Message{
		From:      "Alice",
		Content:   "Hello",
		Timestamp: now,
		IsSystem:  true,
		IsAction:  false,
	}
	
	if msg.From != "Alice" {
		t.Errorf("Expected From='Alice', got %s", msg.From)
	}
	
	if msg.Content != "Hello" {
		t.Errorf("Expected Content='Hello', got %s", msg.Content)
	}
	
	if !msg.IsSystem {
		t.Error("Expected IsSystem=true")
	}
	
	if msg.IsAction {
		t.Error("Expected IsAction=false")
	}
}

func TestRoomChannels(t *testing.T) {
	room := NewRoom("Test Room", 5)
	defer room.Stop()
	
	// Test that channels are properly initialized
	if cap(room.broadcast) != 0 {
		t.Errorf("Expected unbuffered broadcast channel, got capacity %d", cap(room.broadcast))
	}
	
	if cap(room.join) != 0 {
		t.Errorf("Expected unbuffered join channel, got capacity %d", cap(room.join))
	}
	
	if cap(room.leave) != 0 {
		t.Errorf("Expected unbuffered leave channel, got capacity %d", cap(room.leave))
	}
}