package chat

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestChatModelRetainsNewestMessages(t *testing.T) {
	room, model := newChatTestModel(t)
	defer room.Stop()
	model.height = 100
	model.initViewport()

	for i := range maxTUIMessageHistory + 1 {
		updated, _ := model.handleChatMsg(ChatMsg{Message: Message{Content: fmt.Sprintf("message-%02d", i)}})
		model = updated.(ChatModel)
	}

	if len(model.messages) != maxTUIMessageHistory {
		t.Fatalf("retained messages = %d, want %d", len(model.messages), maxTUIMessageHistory)
	}
	if got := model.messages[0].Content; got != "message-01" {
		t.Fatalf("oldest retained message = %q, want %q", got, "message-01")
	}
	if got := model.messages[len(model.messages)-1].Content; got != "message-50" {
		t.Fatalf("newest retained message = %q, want %q", got, "message-50")
	}
	view := model.viewport.View()
	if strings.Contains(view, "message-00") || !strings.Contains(view, "message-50") {
		t.Fatalf("viewport does not reflect retention boundary: %q", view)
	}
}

func TestChatModelSystemMessagesUseRetentionLimit(t *testing.T) {
	room, model := newChatTestModel(t)
	defer room.Stop()
	model.initViewport()

	for i := range maxTUIMessageHistory + 3 {
		model.appendSystemMessage(fmt.Sprintf("system-%02d", i))
	}

	if len(model.messages) != maxTUIMessageHistory {
		t.Fatalf("retained system messages = %d, want %d", len(model.messages), maxTUIMessageHistory)
	}
	if got := model.messages[0].Content; got != "system-03" {
		t.Fatalf("oldest retained system message = %q, want %q", got, "system-03")
	}
}

func TestChatModelJoinedLoadsNewestHistory(t *testing.T) {
	room := NewRoom("Test Room", 10, true, 100, false)
	defer room.Stop()
	client := newRoomTestClient(room, "alice")
	model := NewChatModel(client)

	for i := range maxTUIMessageHistory + 10 {
		room.addToHistory(Message{Content: fmt.Sprintf("history-%02d", i), Timestamp: time.Unix(int64(i), 0)})
	}
	updated, _ := model.handleJoined()
	model = updated.(ChatModel)

	if len(model.messages) != maxTUIMessageHistory {
		t.Fatalf("loaded history = %d messages, want %d", len(model.messages), maxTUIMessageHistory)
	}
	if got := model.messages[0].Content; got != "history-10" {
		t.Fatalf("oldest loaded history = %q, want %q", got, "history-10")
	}
	if got := model.messages[len(model.messages)-1].Content; got != "history-59" {
		t.Fatalf("newest loaded history = %q, want %q", got, "history-59")
	}
}

func TestChatModelRemoteMessagesPreserveFollowTailState(t *testing.T) {
	room, model := newChatTestModel(t)
	defer room.Stop()
	model.height = 8
	model.initViewport()
	for i := range 12 {
		model.appendMessages(Message{Content: fmt.Sprintf("line-%02d", i)})
	}
	model.updateViewportContent()
	model.viewport.GotoBottom()

	updated, _ := model.handleChatMsg(ChatMsg{Message: Message{Content: "at-tail"}})
	model = updated.(ChatModel)
	if !model.viewport.AtBottom() {
		t.Fatal("viewport stopped following the tail after a remote message")
	}

	model.viewport.HalfPageUp()
	if model.viewport.AtBottom() {
		t.Fatal("test viewport did not scroll away from the tail")
	}
	before := model.viewport.YOffset
	updated, _ = model.handleChatMsg(ChatMsg{Message: Message{Content: "while-scrolled"}})
	model = updated.(ChatModel)
	if model.viewport.YOffset != before {
		t.Fatalf("remote message moved scrolled viewport from %d to %d", before, model.viewport.YOffset)
	}
}

func TestChatModelJoinCommandMapsAdmissionErrors(t *testing.T) {
	fullRoom := NewRoom("Full Room", 1, false, 0, false)
	defer fullRoom.Stop()
	first := newRoomTestClient(fullRoom, "alice")
	fullRoom.ReserveNickname(first.Nickname)
	if err := fullRoom.Join(first); err != nil {
		t.Fatalf("join first client: %v", err)
	}
	rejected := newRoomTestClient(fullRoom, "bob")
	fullRoom.ReserveNickname(rejected.Nickname)
	fullModel := NewChatModel(rejected)
	if _, ok := fullModel.joinRoomCmd()().(RoomFullMsg); !ok {
		t.Fatalf("full-room join command did not return RoomFullMsg")
	}

	closedRoom := NewRoom("Closed Room", 1, false, 0, false)
	if err := closedRoom.Stop(); err != nil {
		t.Fatalf("stop closed room: %v", err)
	}
	closedClient := newRoomTestClient(closedRoom, "carol")
	closedRoom.ReserveNickname(closedClient.Nickname)
	closedModel := NewChatModel(closedClient)
	if _, ok := closedModel.joinRoomCmd()().(roomClosedMsg); !ok {
		t.Fatalf("closed-room join command did not return roomClosedMsg")
	}
}

func TestChatModelJoinCommandReleasesDisconnectedClient(t *testing.T) {
	room := NewRoom("Test Room", 1, false, 0, false)
	defer room.Stop()
	client := newRoomTestClient(room, "alice")
	if !room.ReserveNickname(client.Nickname) {
		t.Fatal("ReserveNickname returned false")
	}
	client.disconnected.Store(true)

	model := NewChatModel(client)
	if _, ok := model.joinRoomCmd()().(roomClosedMsg); !ok {
		t.Fatal("disconnected join command did not return roomClosedMsg")
	}

	// A following actor event is a barrier proving the queued Leave completed.
	room.Broadcast(Message{})
	if !room.IsNicknameAvailable(client.Nickname) {
		t.Fatal("disconnected join retained the nickname")
	}
}

func TestChatModelSanitizesStatusOperands(t *testing.T) {
	room := NewRoom("Room\x1b[2J\x07", 10, false, 0, false)
	defer room.Stop()
	client := newRoomTestClient(room, "alice\x1b]0;owned\x07")
	model := NewChatModel(client)
	model.state = stateChat
	model.initViewport()

	view := model.chatView()
	for _, forbidden := range []string{"\x1b[2J", "\x1b]0;owned", "\x07"} {
		if strings.Contains(view, forbidden) {
			t.Fatalf("chat view retained terminal control %q: %q", forbidden, view)
		}
	}
}

func newChatTestModel(t *testing.T) (*Room, ChatModel) {
	t.Helper()
	room := NewRoom("Test Room", 10, false, 0, false)
	client := newRoomTestClient(room, "alice")
	model := NewChatModel(client)
	model.state = stateChat
	return room, model
}
