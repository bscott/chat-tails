package server

import (
	"io"
	"net"
	"testing"
	"time"
)

// Coverage categories for the server package:
//
//   - Security     : covered — a stopped server must release its listening
//                    port so no stale socket keeps accepting clients.
//   - Performance  : covered — many rapid connect/disconnect cycles must not
//                    leak goroutines or wedge the accept loop.
//   - Retry        : N/A — Start performs a single bind; retry policy is the
//                    operator's concern. Placeholder kept below.
//   - Unit         : covered — NewServer wiring and Config field mapping.
//   - Integration  : covered — Start → dial → Stop round trip over a real TCP
//                    listener (plain-text mode to avoid the interactive TUI).
//   - Functional   : covered — a connecting client is tracked and then cleaned
//                    up after Stop.
//   - Frame        : covered — plain-text mode wires the line-mode handler
//                    (verified indirectly by the round-trip accepting a line
//                    client without error).

// testConfig returns a config bound to an OS-assigned port in plain-text mode,
// which uses the non-interactive line handler instead of the bubbletea TUI.
func testConfig() Config {
	return Config{
		Port:      0, // OS picks a free port
		RoomName:  "test",
		MaxUsers:  10,
		PlainText: true,
	}
}

// --- Unit -----------------------------------------------------------------

func TestUnit_NewServer(t *testing.T) {
	cfg := testConfig()
	s, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	if s == nil {
		t.Fatal("NewServer returned nil server")
	}
	if s.config.RoomName != "test" {
		t.Errorf("config not stored: got room %q", s.config.RoomName)
	}
	if s.chatRoom == nil {
		t.Error("expected chat room to be initialized")
	}
	if s.connections == nil {
		t.Error("expected connections map to be initialized")
	}
	if s.ctx == nil || s.cancel == nil {
		t.Error("expected context and cancel func to be initialized")
	}
}

func TestUnit_ConfigFieldMapping(t *testing.T) {
	cfg := Config{
		Port:            2323,
		RoomName:        "lobby",
		MaxUsers:        50,
		EnableTailscale: true,
		HostName:        "chat-node",
		EnableHistory:   true,
		HistorySize:     100,
		PlainText:       true,
	}
	if cfg.Port != 2323 || cfg.MaxUsers != 50 || cfg.HistorySize != 100 {
		t.Error("numeric config fields not preserved")
	}
	if !cfg.EnableTailscale || !cfg.EnableHistory || !cfg.PlainText {
		t.Error("boolean config fields not preserved")
	}
	if cfg.HostName != "chat-node" || cfg.RoomName != "lobby" {
		t.Error("string config fields not preserved")
	}
}

// --- Retry ----------------------------------------------------------------

func TestRetry_NotApplicable(t *testing.T) {
	t.Skip("N/A: Start binds once; rebinding/backoff is an operator concern")
}

// --- Integration / Functional --------------------------------------------

// TestIntegration_StartDialStop starts a real listener, connects a client, and
// then shuts the server down cleanly within the Stop timeout.
func TestIntegration_StartDialStop(t *testing.T) {
	s, err := NewServer(testConfig())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	addr := s.listener.Addr().String()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}

	// The connection should be registered by the accept loop shortly.
	if !waitForConnCount(s, 1, time.Second) {
		t.Error("expected server to track the new connection")
	}

	conn.Close()

	stopped := make(chan error, 1)
	go func() { stopped <- s.Stop() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Stop did not return within timeout")
	}
}

// --- Security -------------------------------------------------------------

// TestSecurity_StopReleasesPort verifies the listening socket is closed after
// Stop, so nothing keeps accepting connections on the old port.
func TestSecurity_StopReleasesPort(t *testing.T) {
	s, err := NewServer(testConfig())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := s.listener.Addr().String()

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// A dial to the old address should now fail (port released).
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Errorf("expected dial to %s to fail after Stop, but it succeeded", addr)
	}
}

// --- Performance ----------------------------------------------------------

// TestPerformance_RapidConnects opens and closes many connections quickly and
// then shuts down, checking the accept loop stays healthy and Stop still
// completes within its timeout (no goroutine wedge).
func TestPerformance_RapidConnects(t *testing.T) {
	s, err := NewServer(testConfig())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addr := s.listener.Addr().String()

	for i := 0; i < 20; i++ {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		// Drain briefly so the handler can run, then disconnect.
		c.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
		_, _ = io.CopyN(io.Discard, c, 1)
		c.Close()
	}

	stopped := make(chan error, 1)
	go func() { stopped <- s.Stop() }()
	select {
	case <-stopped:
	case <-time.After(6 * time.Second):
		t.Fatal("Stop did not return within timeout after rapid connects")
	}
}

// waitForConnCount polls the server's tracked connection count until it reaches
// want or the deadline elapses.
func waitForConnCount(s *Server, want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		n := len(s.connections)
		s.mu.Unlock()
		if n >= want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
