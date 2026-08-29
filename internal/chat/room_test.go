package chat

import (
	"bufio"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestNewRoom(t *testing.T) {
	room := NewRoom("Test Room", 10, false, 0, false)
	t.Cleanup(func() { _ = room.Stop() })

	if room.Name != "Test Room" {
		t.Errorf("room name = %q, want %q", room.Name, "Test Room")
	}
	if room.MaxUsers != 10 {
		t.Errorf("room max users = %d, want 10", room.MaxUsers)
	}
	if room.clients == nil || room.broadcast == nil || room.join == nil || room.leave == nil {
		t.Fatal("room collections were not initialized")
	}
}

func TestRoomChannelsAreUnbuffered(t *testing.T) {
	room := NewRoom("Test Room", 5, false, 0, false)
	t.Cleanup(func() { _ = room.Stop() })

	if cap(room.broadcast) != 0 || cap(room.join) != 0 || cap(room.leave) != 0 {
		t.Fatalf("channel capacities = broadcast %d, join %d, leave %d; want all unbuffered", cap(room.broadcast), cap(room.join), cap(room.leave))
	}
}

func TestRoomJoinReturnsAccepted(t *testing.T) {
	room := NewRoom("Test Room", 1, false, 0, false)
	t.Cleanup(func() { _ = room.Stop() })
	client := newRoomTestClient(room, "alice")

	if !room.ReserveNickname(client.Nickname) {
		t.Fatal("ReserveNickname returned false")
	}
	if err := room.Join(client); err != nil {
		t.Fatalf("Join: %v", err)
	}

	room.mu.RLock()
	joined := room.clients[client.Nickname]
	room.mu.RUnlock()
	if joined != client {
		t.Fatalf("joined client = %p, want %p", joined, client)
	}
}

func TestRoomJoinReturnsRoomFull(t *testing.T) {
	room := NewRoom("Test Room", 1, false, 0, false)
	t.Cleanup(func() { _ = room.Stop() })
	first := newRoomTestClient(room, "alice")
	second := newRoomTestClient(room, "bob")

	room.ReserveNickname(first.Nickname)
	if err := room.Join(first); err != nil {
		t.Fatalf("join first client: %v", err)
	}
	room.ReserveNickname(second.Nickname)
	if err := room.Join(second); !errors.Is(err, ErrRoomFull) {
		t.Fatalf("join second client error = %v, want ErrRoomFull", err)
	}

	room.mu.RLock()
	firstEntry := room.clients[first.Nickname]
	_, secondExists := room.clients[second.Nickname]
	room.mu.RUnlock()
	if firstEntry != first {
		t.Fatalf("first client was replaced: got %p, want %p", firstEntry, first)
	}
	if secondExists {
		t.Fatal("rejected client's reservation was not released")
	}
}

func TestRoomJoinAfterStopReturnsClosed(t *testing.T) {
	room := NewRoom("Test Room", 1, false, 0, false)
	if err := room.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	client := newRoomTestClient(room, "alice")
	room.ReserveNickname(client.Nickname)
	if err := room.Join(client); !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("Join after Stop error = %v, want ErrRoomClosed", err)
	}
	if !room.IsNicknameAvailable(client.Nickname) {
		t.Fatal("Join after Stop left a nickname reservation")
	}
}

func TestRoomStopIsConcurrentAndIdempotent(t *testing.T) {
	room := NewRoom("Test Room", 1, false, 0, false)
	const callers = 8
	start := make(chan struct{})
	results := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			results <- room.Stop()
		}()
	}
	close(start)

	for range callers {
		select {
		case err := <-results:
			if err != nil {
				t.Errorf("Stop returned %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent Stop blocked")
		}
	}
}

func TestRoomProducersAfterStopReturn(t *testing.T) {
	room := NewRoom("Test Room", 1, false, 0, false)
	client := newRoomTestClient(room, "alice")
	if err := room.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	returned := make(chan struct{}, 2)
	go func() {
		room.Broadcast(Message{Content: "hello"})
		returned <- struct{}{}
	}()
	go func() {
		room.Leave(client)
		returned <- struct{}{}
	}()
	for range 2 {
		select {
		case <-returned:
		case <-time.After(time.Second):
			t.Fatal("producer blocked after room shutdown")
		}
	}
	if err := room.Join(client); !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("Join after Stop error = %v, want ErrRoomClosed", err)
	}
}

func TestRoomStaleLeaveDoesNotRemoveReplacement(t *testing.T) {
	room := NewRoom("Test Room", 2, false, 0, false)
	t.Cleanup(func() { _ = room.Stop() })
	oldClient := newRoomTestClient(room, "alice")
	replacement := newRoomTestClient(room, "alice")

	room.mu.Lock()
	room.clients[oldClient.Nickname] = replacement
	room.mu.Unlock()

	room.removeClient(oldClient)
	room.removeClient(oldClient)

	room.mu.RLock()
	got := room.clients[oldClient.Nickname]
	room.mu.RUnlock()
	if got != replacement {
		t.Fatalf("stale Leave left client %p, want replacement %p", got, replacement)
	}
}

func TestMessageStruct(t *testing.T) {
	now := time.Now()
	msg := Message{From: "Alice", Content: "Hello", Timestamp: now, IsSystem: true}
	if msg.From != "Alice" || msg.Content != "Hello" || !msg.IsSystem || msg.IsAction {
		t.Fatalf("unexpected message: %#v", msg)
	}
}

func newRoomTestClient(room *Room, nickname string) *Client {
	return &Client{
		Nickname: nickname,
		conn:     discardConn{},
		writer:   bufio.NewWriter(io.Discard),
		room:     room,
	}
}

type discardConn struct{}

func (discardConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (discardConn) Write(p []byte) (int, error)      { return len(p), nil }
func (discardConn) Close() error                     { return nil }
func (discardConn) LocalAddr() net.Addr              { return testAddr("local") }
func (discardConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (discardConn) SetDeadline(time.Time) error      { return nil }
func (discardConn) SetReadDeadline(time.Time) error  { return nil }
func (discardConn) SetWriteDeadline(time.Time) error { return nil }

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }
