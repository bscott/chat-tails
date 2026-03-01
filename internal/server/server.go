package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/bscott/ts-chat/internal/chat"
	"tailscale.com/tsnet"
)

// Server represents the chat server
type Server struct {
	config      Config
	listener    net.Listener
	tsServer    *tsnet.Server
	chatRoom    *chat.Room
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	connections map[string]net.Conn
	mu          sync.Mutex
}

// NewServer creates a new chat server
func NewServer(cfg Config) (*Server, error) {
	ctx, cancel := context.WithCancel(context.Background())

	room := chat.NewRoom(cfg.RoomName, cfg.MaxUsers, cfg.EnableHistory, cfg.HistorySize, cfg.PlainText)

	return &Server{
		config:      cfg,
		ctx:         ctx,
		cancel:      cancel,
		chatRoom:    room,
		connections: make(map[string]net.Conn),
	}, nil
}

// Start starts the chat server
func (s *Server) Start() error {
	var listener net.Listener
	var err error

	if s.config.EnableTailscale {
		s.tsServer = &tsnet.Server{
			Hostname: s.config.HostName,
			AuthKey:  os.Getenv("TS_AUTHKEY"),
		}

		log.Printf("Connecting to Tailscale network...")
		if _, err := s.tsServer.Up(s.ctx); err != nil {
			return fmt.Errorf("failed to start Tailscale node: %w", err)
		}

		ln, err := s.tsServer.LocalClient()
		if err != nil {
			log.Printf("Warning: unable to get Tailscale local client: %v", err)
		} else {
			status, err := ln.Status(s.ctx)
			if err != nil {
				log.Printf("Warning: unable to get Tailscale status: %v", err)
			} else if status != nil && status.Self != nil && status.Self.DNSName != "" {
				log.Printf("Tailscale node running as: %s", status.Self.DNSName)
			} else {
				log.Printf("Tailscale node running but DNS name not available yet")
			}
		}

		listener, err = s.tsServer.Listen("tcp", fmt.Sprintf(":%d", s.config.Port))
		if err != nil {
			return fmt.Errorf("failed to start Tailscale server on port %d: %w", s.config.Port, err)
		}
	} else {
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", s.config.Port))
		if err != nil {
			return fmt.Errorf("failed to listen on port %d: %w", s.config.Port, err)
		}
	}

	s.listener = listener

	log.Printf("Server started on port %d (room: %s, max users: %d)", s.config.Port, s.config.RoomName, s.config.MaxUsers)

	s.wg.Add(1)
	go s.acceptConnections()

	return nil
}

func (s *Server) acceptConnections() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.ctx.Done():
					return
				default:
					log.Printf("Error accepting connection: %v", err)
					continue
				}
			}

			s.wg.Add(1)
			go s.handleConnection(conn)
		}
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	log.Printf("New connection from %s", remoteAddr)

	s.mu.Lock()
	s.connections[remoteAddr] = conn
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.connections, remoteAddr)
		s.mu.Unlock()
		log.Printf("Connection from %s closed", remoteAddr)
	}()

	if s.config.PlainText {
		s.handlePlainText(conn)
	} else {
		s.handleTUI(conn)
	}
}

// handleTUI runs a bubbletea program for the connection.
func (s *Server) handleTUI(conn net.Conn) {
	client := chat.NewTUIClient(conn, s.chatRoom)

	client.RunTUI(s.ctx)

	// Leave room on disconnect if nickname was set
	if client.Nickname != "" {
		s.chatRoom.Leave(client)
	}
}

// handlePlainText uses the legacy line-mode handler.
func (s *Server) handlePlainText(conn net.Conn) {
	client, err := chat.NewPlainTextClient(conn, s.chatRoom)
	if err != nil {
		log.Printf("Error creating client: %v", err)
		return
	}

	client.Handle(s.ctx)
}

// Stop stops the chat server
func (s *Server) Stop() error {
	log.Print("Stopping chat server...")

	s.cancel()

	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			log.Printf("Error closing listener: %v", err)
		}
	}

	s.mu.Lock()
	for _, conn := range s.connections {
		conn.Close()
	}
	s.mu.Unlock()

	if s.chatRoom != nil {
		if err := s.chatRoom.Stop(); err != nil {
			log.Printf("Error stopping chat room: %v", err)
		}
	}

	if s.config.EnableTailscale && s.tsServer != nil {
		if err := s.tsServer.Close(); err != nil {
			log.Printf("Error closing Tailscale node: %v", err)
		}
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Print("Chat server stopped")
	case <-time.After(5 * time.Second):
		log.Print("Chat server stopped (timeout waiting for goroutines)")
	}

	return nil
}

