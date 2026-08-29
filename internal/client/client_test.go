package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

const testTimeout = 2 * time.Second

func TestIACEscapeWriter(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []byte
	}{
		{name: "no IAC", input: []byte("hello"), want: []byte("hello")},
		{name: "lone IAC", input: []byte{255}, want: []byte{255, 255}},
		{name: "IAC mid-stream", input: []byte{'a', 255, 'b'}, want: []byte{'a', 255, 255, 'b'}},
		{name: "IAC at both ends", input: []byte{255, 'x', 255}, want: []byte{255, 255, 'x', 255, 255}},
		{name: "consecutive IACs", input: []byte{255, 255}, want: []byte{255, 255, 255, 255}},
		{name: "empty", input: []byte{}, want: []byte{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			writer := &iacEscapeWriter{writer: &out}
			n, err := writer.Write(tt.input)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if n != len(tt.input) {
				t.Fatalf("Write consumed %d bytes, want %d", n, len(tt.input))
			}
			if !bytes.Equal(out.Bytes(), tt.want) {
				t.Fatalf("escaped output = %v, want %v", out.Bytes(), tt.want)
			}
		})
	}
}

func TestIACEscapeWriterAcrossWriteBoundaries(t *testing.T) {
	var out bytes.Buffer
	writer := &iacEscapeWriter{writer: &out}
	for _, fragment := range [][]byte{{'a', 255}, {255, 'b'}} {
		if _, err := writer.Write(fragment); err != nil {
			t.Fatalf("Write(%v): %v", fragment, err)
		}
	}
	want := []byte{'a', 255, 255, 255, 255, 'b'}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("escaped output = %v, want %v", out.Bytes(), want)
	}
}

type shortWriter struct {
	out bytes.Buffer
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return w.out.Write(p)
}

func TestIACEscapeWriterHandlesShortWrites(t *testing.T) {
	out := &shortWriter{}
	writer := &iacEscapeWriter{writer: out}
	input := []byte{'a', 'b', 255, 'c'}
	n, err := writer.Write(input)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(input) {
		t.Fatalf("Write consumed %d bytes, want %d", n, len(input))
	}
	if want := []byte{'a', 'b', 255, 255, 'c'}; !bytes.Equal(out.out.Bytes(), want) {
		t.Fatalf("escaped output = %v, want %v", out.out.Bytes(), want)
	}
}

// startRun launches Run with piped local input and a buffered local output,
// returning the server side of the connection and the plumbing handles.
func startRun(t *testing.T, ctx context.Context) (server net.Conn, in io.WriteCloser, out *bytes.Buffer, done chan error) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	inReader, inWriter := io.Pipe()
	out = &bytes.Buffer{}
	done = make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Conn: clientSide, In: inReader, Out: out})
	}()
	t.Cleanup(func() {
		serverSide.Close()
		inWriter.Close()
		waitDone(t, done)
	})
	return serverSide, inWriter, out, done
}

func waitDone(t *testing.T, done chan error) error {
	t.Helper()
	select {
	case err := <-done:
		done <- err // keep the result readable for the test body
		return err
	case <-time.After(testTimeout):
		t.Fatal("Run did not return")
		return nil
	}
}

func TestRunFiltersServerOutputAndEscapesInput(t *testing.T) {
	server, in, out, done := startRun(t, context.Background())

	// Telnet negotiation followed by payload must reach Out as payload only.
	// net.Pipe writes are synchronous, so these complete once Run consumed them.
	if _, err := server.Write([]byte{255, 251, 1, 255, 251, 3, 255, 253, 3}); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	if _, err := server.Write([]byte("hello")); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	// Local input with a literal 0xff must arrive IAC-escaped.
	if _, err := in.Write([]byte("hi\xff!")); err != nil {
		t.Fatalf("write input: %v", err)
	}
	received := make([]byte, 5)
	if _, err := io.ReadFull(server, received); err != nil {
		t.Fatalf("read relayed input: %v", err)
	}
	if want := []byte("hi\xff\xff!"); !bytes.Equal(received, want) {
		t.Fatalf("server received %v, want %v", received, want)
	}

	// Server-side close ends the session cleanly.
	server.Close()
	if err := waitDone(t, done); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := out.String(); got != "hello" {
		t.Fatalf("local output = %q, want %q (negotiation must be filtered)", got, "hello")
	}
}

func TestRunReturnsNilOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, _, _, done := startRun(t, ctx)

	cancel()
	if err := waitDone(t, done); err != nil {
		t.Fatalf("Run after cancel: %v", err)
	}
}

func TestRunKeepsReadingAfterLocalInputEOF(t *testing.T) {
	server, in, out, done := startRun(t, context.Background())

	if err := in.Close(); err != nil {
		t.Fatalf("close input: %v", err)
	}
	if _, err := server.Write([]byte("late")); err != nil {
		t.Fatalf("write after input EOF: %v", err)
	}
	server.Close()
	if err := waitDone(t, done); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := out.String(); got != "late" {
		t.Fatalf("local output = %q, want %q", got, "late")
	}
}

// failWriteConn fails every write so the input-relay error path is reachable.
type failWriteConn struct {
	net.Conn
}

var errWriteRefused = errors.New("write refused")

func (failWriteConn) Write([]byte) (int, error) { return 0, errWriteRefused }

func TestRunReportsInputRelayFailure(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	inReader, inWriter := io.Pipe()
	defer inWriter.Close()

	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Config{
			Conn: failWriteConn{Conn: clientSide},
			In:   inReader,
			Out:  io.Discard,
		})
	}()

	if _, err := inWriter.Write([]byte("x")); err != nil {
		t.Fatalf("write input: %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, errWriteRefused) || !strings.Contains(err.Error(), "relay input") {
			t.Fatalf("Run error = %v, want wrapped %v", err, errWriteRefused)
		}
	case <-time.After(testTimeout):
		t.Fatal("Run did not return after input relay failure")
	}
}
