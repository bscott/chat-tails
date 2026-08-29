package chat

import (
	"bytes"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientConstants(t *testing.T) {
	if MaxMessageLength <= 0 || MaxMessageLength > 10000 {
		t.Errorf("MaxMessageLength is unreasonable: %d", MaxMessageLength)
	}
	if MessageRateLimit <= 0 {
		t.Errorf("MessageRateLimit should be positive, got %d", MessageRateLimit)
	}
	if RateLimitWindow <= 0 {
		t.Errorf("RateLimitWindow should be positive, got %v", RateLimitWindow)
	}
}

func TestMessageValidation(t *testing.T) {
	tests := []struct {
		name       string
		messageLen int
		valid      bool
	}{
		{name: "short message", messageLen: 10, valid: true},
		{name: "medium message", messageLen: 500, valid: true},
		{name: "max length", messageLen: MaxMessageLength, valid: true},
		{name: "over max", messageLen: MaxMessageLength + 1, valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{}
			err := client.validateMessageLength(strings.Repeat("a", tt.messageLen))
			if tt.valid && err != nil {
				t.Fatalf("validateMessageLength returned %v", err)
			}
			if !tt.valid && err == nil {
				t.Fatal("validateMessageLength accepted an oversized message")
			}
		})
	}
}

func TestTelnetFilterReaderStreaming(t *testing.T) {
	const iac = byte(255)
	tests := []struct {
		name      string
		fragments [][]byte
		want      []byte
	}{
		{
			name:      "ordinary nc input",
			fragments: [][]byte{[]byte("alice\r\n")},
			want:      []byte("alice\r\n"),
		},
		{
			name:      "negotiation followed immediately by input",
			fragments: [][]byte{{iac, 251, 1}, []byte("hello")},
			want:      []byte("hello"),
		},
		{
			name:      "fragmented option command",
			fragments: [][]byte{[]byte("a"), {iac}, {251}, {1}, []byte("b")},
			want:      []byte("ab"),
		},
		{
			name:      "fragmented escaped IAC",
			fragments: [][]byte{[]byte("a"), {iac}, {iac}, []byte("b")},
			want:      []byte{'a', iac, 'b'},
		},
		{
			name: "fragmented subnegotiation",
			fragments: [][]byte{
				[]byte("a"), {iac}, {250}, {24}, []byte("payload"), {iac}, {240}, []byte("b"),
			},
			want: []byte("ab"),
		},
		{
			name: "escaped IAC inside subnegotiation",
			fragments: [][]byte{
				{iac, 250, 24}, {'x', iac}, {iac, 'y'}, {iac, 240}, []byte("ok"),
			},
			want: []byte("ok"),
		},
		{
			name:      "two byte command",
			fragments: [][]byte{[]byte("a"), {iac, 241}, []byte("b")},
			want:      []byte("ab"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := &telnetFilterReader{reader: &fragmentReader{fragments: tt.fragments}}
			got, err := io.ReadAll(filtered)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("filtered bytes = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTelnetFilterReaderFiltersEveryOptionCommand(t *testing.T) {
	for _, command := range []byte{251, 252, 253, 254} {
		filtered := &telnetFilterReader{
			reader: &fragmentReader{fragments: [][]byte{
				[]byte("a"),
				{255},
				{command},
				{1},
				[]byte("b"),
			}},
		}
		got, err := io.ReadAll(filtered)
		if err != nil {
			t.Fatalf("command %d: ReadAll: %v", command, err)
		}
		if string(got) != "ab" {
			t.Fatalf("command %d: filtered output = %q, want %q", command, got, "ab")
		}
	}
}

func TestTelnetFilterReaderSkipsNegotiationOnlyReads(t *testing.T) {
	filtered := &telnetFilterReader{reader: &fragmentReader{fragments: [][]byte{{255, 251, 1}, []byte("text")}}}
	buffer := make([]byte, 8)
	n, err := filtered.Read(buffer)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n == 0 {
		t.Fatal("Read returned (0, nil) after a negotiation-only fragment")
	}
	if got := string(buffer[:n]); got != "text" {
		t.Fatalf("Read returned %q, want %q", got, "text")
	}
}

func TestTelnetFilterReaderPreservesOutputWithTinyBuffers(t *testing.T) {
	filtered := &telnetFilterReader{reader: strings.NewReader("abcdef")}
	var got bytes.Buffer
	buffer := make([]byte, 1)
	for {
		n, err := filtered.Read(buffer)
		got.Write(buffer[:n])
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}
	if got.String() != "abcdef" {
		t.Fatalf("filtered output = %q, want %q", got.String(), "abcdef")
	}
}

func TestTelnetFilterReaderDefersEOFUntilDataIsReturned(t *testing.T) {
	filtered := &telnetFilterReader{reader: &dataEOFReader{data: []byte("hello")}}
	buffer := make([]byte, 8)
	n, err := filtered.Read(buffer)
	if err != nil || string(buffer[:n]) != "hello" {
		t.Fatalf("first Read = (%q, %v), want (%q, nil)", buffer[:n], err, "hello")
	}
	n, err = filtered.Read(buffer)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("second Read = (%d, %v), want (0, EOF)", n, err)
	}
}

func TestTelnetFilterReaderDropsIncompleteNegotiationAtEOF(t *testing.T) {
	for _, input := range [][]byte{{255}, {255, 251}, {255, 250, 24, 'x'}} {
		filtered := &telnetFilterReader{reader: bytes.NewReader(input)}
		got, err := io.ReadAll(filtered)
		if err != nil {
			t.Fatalf("ReadAll(%v): %v", input, err)
		}
		if len(got) != 0 {
			t.Fatalf("ReadAll(%v) returned protocol bytes %v", input, got)
		}
	}
}

func TestNewPlainTextClientReturnsRoomFull(t *testing.T) {
	room := NewRoom("Test Room", 1, false, 0, true)
	t.Cleanup(func() { _ = room.Stop() })
	first := newRoomTestClient(room, "alice")
	room.ReserveNickname(first.Nickname)
	if err := room.Join(first); err != nil {
		t.Fatalf("join first client: %v", err)
	}

	conn := newScriptedConn("bob\n")
	client, err := NewPlainTextClient(conn, room)
	if client != nil {
		t.Fatalf("rejected client = %#v, want nil", client)
	}
	if !errors.Is(err, ErrRoomFull) {
		t.Fatalf("NewPlainTextClient error = %v, want ErrRoomFull", err)
	}
	if !conn.isClosed() {
		t.Fatal("rejected connection was not closed")
	}
	output := conn.outputString()
	if !strings.Contains(output, "Sorry, the room is full. Try again later.") {
		t.Fatalf("rejection output missing room-full notice: %q", output)
	}
	if strings.Contains(output, "Welcome to Test Room, bob!") {
		t.Fatalf("rejected client received post-admission welcome: %q", output)
	}
}

type fragmentReader struct {
	fragments [][]byte
	index     int
	offset    int
}

func (r *fragmentReader) Read(p []byte) (int, error) {
	if r.index >= len(r.fragments) {
		return 0, io.EOF
	}
	fragment := r.fragments[r.index]
	n := copy(p, fragment[r.offset:])
	r.offset += n
	if r.offset == len(fragment) {
		r.index++
		r.offset = 0
	}
	return n, nil
}

type dataEOFReader struct {
	data []byte
	done bool
}

func (r *dataEOFReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, r.data), io.EOF
}

type scriptedConn struct {
	input  *strings.Reader
	output bytes.Buffer
	closed bool
	mu     sync.Mutex
}

func newScriptedConn(input string) *scriptedConn {
	return &scriptedConn{input: strings.NewReader(input)}
}

func (c *scriptedConn) Read(p []byte) (int, error) {
	return c.input.Read(p)
}

func (c *scriptedConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.Write(p)
}

func (c *scriptedConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *scriptedConn) outputString() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.output.String()
}

func (c *scriptedConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (*scriptedConn) LocalAddr() net.Addr              { return testAddr("local") }
func (*scriptedConn) RemoteAddr() net.Addr             { return testAddr("remote") }
func (*scriptedConn) SetDeadline(time.Time) error      { return nil }
func (*scriptedConn) SetReadDeadline(time.Time) error  { return nil }
func (*scriptedConn) SetWriteDeadline(time.Time) error { return nil }
