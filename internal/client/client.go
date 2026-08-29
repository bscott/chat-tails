// Package client implements the first-party terminal client: a byte relay
// between the local terminal and the server-rendered TUI. It speaks no
// protocol of its own beyond telnet IAC hygiene — the server draws the UI.
package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"

	"github.com/bscott/ts-chat/internal/chat"
)

// Config wires an established server connection to the local terminal streams.
type Config struct {
	Conn net.Conn  // established connection to the chat server
	In   io.Reader // local input, normally a raw-mode stdin
	Out  io.Writer // local output, normally stdout
}

// Run relays cfg.In to the server and filtered server output to cfg.Out until
// the server disconnects, the context is cancelled, or relaying fails.
// Incoming telnet IAC sequences are stripped; outbound 0xff bytes are escaped
// as IAC IAC. Cancellation and a server-side close both return nil.
func Run(ctx context.Context, cfg Config) error {
	conn := cfg.Conn
	defer conn.Close()

	stopClose := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClose()

	inputErr := make(chan error, 1)
	go func() {
		_, err := io.Copy(&iacEscapeWriter{writer: conn}, cfg.In)
		if err != nil {
			inputErr <- err
			conn.Close()
			return
		}
		// Local input ended; half-close where possible so the server sees
		// EOF while its remaining output still reaches us.
		if halfCloser, ok := conn.(interface{ CloseWrite() error }); ok {
			halfCloser.CloseWrite()
		}
	}()

	_, readErr := io.Copy(cfg.Out, chat.NewTelnetFilterReader(conn))

	if ctx.Err() != nil {
		return nil
	}
	select {
	case err := <-inputErr:
		return fmt.Errorf("relay input: %w", err)
	default:
	}
	if readErr != nil {
		return fmt.Errorf("read from server: %w", readErr)
	}
	return nil
}

const iacByte = byte(255)

var iacEscape = []byte{iacByte}

// iacEscapeWriter escapes literal 0xff bytes as IAC IAC so the server's
// telnet filter does not treat them as the start of a command.
type iacEscapeWriter struct {
	writer io.Writer
}

// Write reports the number of bytes consumed from p, per io.Writer; escape
// bytes written downstream are not counted.
func (w *iacEscapeWriter) Write(p []byte) (int, error) {
	consumed := 0
	for len(p) > 0 {
		i := bytes.IndexByte(p, iacByte)
		if i < 0 {
			n, err := writeAll(w.writer, p)
			return consumed + n, err
		}

		n, err := writeAll(w.writer, p[:i+1])
		consumed += n
		if err != nil {
			return consumed, err
		}
		if _, err := writeAll(w.writer, iacEscape); err != nil {
			return consumed, err
		}
		p = p[i+1:]
	}
	return consumed, nil
}

func writeAll(w io.Writer, p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n, err := w.Write(p)
		written += n
		p = p[n:]
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}
