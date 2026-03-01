package chat

// ChatMsg is a tea.Msg sent when a chat message arrives from the room broadcast.
type ChatMsg struct {
	Message
}

// JoinedMsg indicates the client successfully joined the room.
type JoinedMsg struct{}

// RoomFullMsg indicates the room is full and the client was rejected.
type RoomFullMsg struct{}
