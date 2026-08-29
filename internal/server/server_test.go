package server

import (
	"bufio"
	"errors"
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bscott/ts-chat/internal/chat"
)

const testTimeout = 2 * time.Second

func testConfig() Config {
	return Config{
		Port:          0,
		RoomName:      "test room",
		MaxUsers:      10,
		EnableHistory: true,
		HistorySize:   5,
		PlainText:     true,
	}
}

func TestNewServerWiresConfigAndRoom(t *testing.T) {
	cfg := testConfig()
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = s.Stop() })

	if !reflect.DeepEqual(s.config, cfg) {
		t.Fatalf("stored config = %#v, want %#v", s.config, cfg)
	}
	if s.chatRoom == nil {
		t.Fatal("NewServer did not create a chat room")
	}
	if s.chatRoom.Name != cfg.RoomName {
		t.Errorf("room name = %q, want %q", s.chatRoom.Name, cfg.RoomName)
	}
	if s.chatRoom.MaxUsers != cfg.MaxUsers {
		t.Errorf("room max users = %d, want %d", s.chatRoom.MaxUsers, cfg.MaxUsers)
	}
	if s.chatRoom.PlainText != cfg.PlainText {
		t.Errorf("room plain-text mode = %t, want %t", s.chatRoom.PlainText, cfg.PlainText)
	}
}

func TestNewServerRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "negative port", mutate: func(c *Config) { c.Port = -1 }, wantErr: "port must be between 0 and 65535"},
		{name: "port above maximum", mutate: func(c *Config) { c.Port = 65536 }, wantErr: "port must be between 0 and 65535"},
		{name: "blank room name", mutate: func(c *Config) { c.RoomName = " \t" }, wantErr: "room name must not be empty"},
		{name: "zero max users", mutate: func(c *Config) { c.MaxUsers = 0 }, wantErr: "max users must be greater than zero"},
		{name: "negative max users", mutate: func(c *Config) { c.MaxUsers = -1 }, wantErr: "max users must be greater than zero"},
		{name: "negative history size", mutate: func(c *Config) { c.HistorySize = -1 }, wantErr: "history size must not be negative"},
		{
			name: "zero enabled history size",
			mutate: func(c *Config) {
				c.EnableHistory = true
				c.HistorySize = 0
			},
			wantErr: "history size must be greater than zero when history is enabled",
		},
		{
			name: "blank Tailscale hostname",
			mutate: func(c *Config) {
				c.EnableTailscale = true
				c.HostName = " \t"
			},
			wantErr: "hostname must not be empty when Tailscale is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig()
			tt.mutate(&cfg)
			server, err := NewServer(cfg)
			if server != nil {
				t.Cleanup(func() { _ = server.Stop() })
				t.Fatalf("NewServer returned a server for invalid config")
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("NewServer error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestNewServerAcceptsConfigBoundaries(t *testing.T) {
	tests := []Config{
		{Port: 0, RoomName: "room", MaxUsers: 1, HistorySize: 0},
		{Port: 65535, RoomName: "room", MaxUsers: 1, HistorySize: 1},
		{Port: 1, RoomName: "room", MaxUsers: 1, HistorySize: 1, HostName: ""},
		{Port: 0, RoomName: "room", MaxUsers: 1, HistorySize: 0, EnableTailscale: true, HostName: "chat"},
	}
	for _, cfg := range tests {
		server, err := NewServer(cfg)
		if err != nil {
			t.Fatalf("NewServer(%#v): %v", cfg, err)
		}
		if err := server.Stop(); err != nil {
			t.Fatalf("Stop: %v", err)
		}
	}
}

func TestConnectionAddressLifecycle(t *testing.T) {
	s, err := NewServer(testConfig())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = s.Stop() })

	if host, port, ok := s.ConnectionAddress(); ok || host != "" || port != 0 {
		t.Fatalf("address before Start = (%q, %d, %t), want unavailable", host, port, ok)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	host, port, ok := s.ConnectionAddress()
	if !ok || host != "localhost" {
		t.Fatalf("address after Start = (%q, %d, %t), want localhost and available", host, port, ok)
	}
	listenerAddress, ok := s.listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address has type %T, want *net.TCPAddr", s.listener.Addr())
	}
	actualPort := listenerAddress.Port
	if port == 0 || port != actualPort {
		t.Fatalf("reported port = %d, actual port = %d", port, actualPort)
	}
}

func TestNormalizeDNSName(t *testing.T) {
	tests := map[string]string{
		"chat.example.ts.net.": "chat.example.ts.net",
		"chat.example.ts.net":  "chat.example.ts.net",
		"":                     "",
		".":                    "",
	}
	for input, want := range tests {
		if got := normalizeDNSName(input); got != want {
			t.Errorf("normalizeDNSName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPlainTextProtocolAndConnectionCleanup(t *testing.T) {
	s, stop := startTestServer(t)
	conn := dialServer(t, s)
	reader := bufio.NewReader(conn)

	readUntil(t, conn, reader, "Please enter your nickname: ")
	writeString(t, conn, "alice\n")

	welcome := readUntil(t, conn, reader, "> ")
	if !strings.Contains(welcome, "Welcome to test room, alice!") {
		t.Fatalf("welcome output did not contain greeting:\n%s", welcome)
	}

	writeString(t, conn, "hello everyone\n")
	message := readUntil(t, conn, reader, "alice: hello everyone")
	if !strings.Contains(message, "alice: hello everyone") {
		t.Fatalf("message output did not contain broadcast:\n%s", message)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	if !waitForConnectionCount(s, 0, testTimeout) {
		t.Fatalf("tracked connections = %d after client disconnect, want 0", connectionCount(s))
	}
	if err := stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStopClosesActiveConnections(t *testing.T) {
	s, stop := startTestServer(t)
	conn := dialServer(t, s)
	reader := bufio.NewReader(conn)

	readUntil(t, conn, reader, "Please enter your nickname: ")
	if !waitForConnectionCount(s, 1, testTimeout) {
		t.Fatalf("tracked connections = %d, want 1", connectionCount(s))
	}

	if err := stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := connectionCount(s); got != 0 {
		t.Fatalf("tracked connections after Stop = %d, want 0", got)
	}

	if err := conn.SetReadDeadline(time.Now().Add(testTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("client connection remained readable after Stop")
	}
}

func TestStopIsConcurrentAndDrainsHandlersBeforeRoom(t *testing.T) {
	s, _ := startTestServer(t)
	conn := dialServer(t, s)
	reader := bufio.NewReader(conn)
	readUntil(t, conn, reader, "Please enter your nickname: ")
	writeString(t, conn, "alice\n")
	readUntil(t, conn, reader, "> ")

	const callers = 8
	start := make(chan struct{})
	results := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			results <- s.Stop()
		}()
	}
	close(start)

	for range callers {
		select {
		case err := <-results:
			if err != nil {
				t.Errorf("Stop returned %v", err)
			}
		case <-time.After(testTimeout):
			t.Fatal("concurrent Stop blocked")
		}
	}
	if got := connectionCount(s); got != 0 {
		t.Fatalf("tracked connections after Stop = %d, want 0", got)
	}
	serverConn, peerConn := net.Pipe()
	defer serverConn.Close()
	defer peerConn.Close()
	client := chat.NewTUIClient(serverConn, s.chatRoom)
	client.Nickname = "after-stop"
	if err := s.chatRoom.Join(client); !errors.Is(err, chat.ErrRoomClosed) {
		t.Fatalf("room Join after server Stop error = %v, want ErrRoomClosed", err)
	}
}

func TestStopReleasesListenerAddress(t *testing.T) {
	s, stop := startTestServer(t)
	network := s.listener.Addr().Network()
	address := s.listener.Addr().String()

	if err := stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	listener, err := net.Listen(network, address)
	if err != nil {
		t.Fatalf("listen on released address %s: %v", address, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close rebound listener: %v", err)
	}
}

func startTestServer(t *testing.T) (*Server, func() error) {
	t.Helper()

	s, err := NewServer(testConfig())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stop := s.Stop
	t.Cleanup(func() { _ = stop() })

	return s, stop
}

func dialServer(t *testing.T, s *Server) net.Conn {
	t.Helper()

	listenerAddr, ok := s.listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address has type %T, want *net.TCPAddr", s.listener.Addr())
	}

	host := net.IPv4(127, 0, 0, 1)
	if listenerAddr.IP != nil && !listenerAddr.IP.IsUnspecified() {
		host = listenerAddr.IP
	} else if listenerAddr.IP != nil && listenerAddr.IP.To4() == nil {
		host = net.IPv6loopback
	}
	address := net.JoinHostPort(host.String(), strconv.Itoa(listenerAddr.Port))

	conn, err := net.DialTimeout("tcp", address, testTimeout)
	if err != nil {
		t.Fatalf("dial %s: %v", address, err)
	}
	return conn
}

func readUntil(t *testing.T, conn net.Conn, reader *bufio.Reader, marker string) string {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(testTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	defer conn.SetReadDeadline(time.Time{})

	var output strings.Builder
	for !strings.HasSuffix(output.String(), marker) {
		if output.Len() > 64*1024 {
			t.Fatalf("server output exceeded limit while waiting for %q", marker)
		}
		b, err := reader.ReadByte()
		if err != nil {
			t.Fatalf("read while waiting for %q: %v\noutput:\n%s", marker, err, output.String())
		}
		output.WriteByte(b)
	}
	return output.String()
}

func writeString(t *testing.T, conn net.Conn, value string) {
	t.Helper()
	if _, err := io.WriteString(conn, value); err != nil {
		t.Fatalf("write %q: %v", value, err)
	}
}

func waitForConnectionCount(s *Server, want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if connectionCount(s) == want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return connectionCount(s) == want
}

func connectionCount(s *Server) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.connections)
}
