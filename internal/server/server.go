package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bscott/ts-chat/internal/chat"
	"tailscale.com/tsnet"
)

// Server represents the chat server
type Server struct {
	config         Config
	listener       net.Listener
	tsServer       *tsnet.Server
	chatRoom       *chat.Room
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	connections    map[string]net.Conn
	mu             sync.Mutex
	endpointMu     sync.RWMutex
	connectionHost string
	connectionPort int
	stopOnce       sync.Once
	stopDone       chan struct{}
	stopErr        error
}

// NewServer creates a new chat server
func NewServer(cfg Config) (*Server, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	room := chat.NewRoom(cfg.RoomName, cfg.MaxUsers, cfg.EnableHistory, cfg.HistorySize, cfg.PlainText)

	return &Server{
		config:      cfg,
		ctx:         ctx,
		cancel:      cancel,
		chatRoom:    room,
		connections: make(map[string]net.Conn),
		stopDone:    make(chan struct{}),
	}, nil
}

// Start starts the chat server
func (s *Server) Start() error {
	var listener net.Listener
	var connectionHost string
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

		localClient, err := s.tsServer.LocalClient()
		if err != nil {
			log.Printf("Warning: unable to get Tailscale local client: %v", err)
		} else {
			status, err := localClient.Status(s.ctx)
			if err != nil {
				log.Printf("Warning: unable to get Tailscale status: %v", err)
			} else if status != nil && status.Self != nil {
				connectionHost = normalizeDNSName(status.Self.DNSName)
			}
			if connectionHost == "" {
				log.Printf("Tailscale node is running, but its DNS name is not available")
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
		connectionHost = "localhost"
	}

	s.listener = listener
	connectionPort, err := listenerPort(listener)
	if err != nil {
		listener.Close()
		return fmt.Errorf("failed to determine listener port: %w", err)
	}
	s.endpointMu.Lock()
	s.connectionHost = connectionHost
	s.connectionPort = connectionPort
	s.endpointMu.Unlock()

	log.Printf("Server started on port %d (room: %s, max users: %d)", connectionPort, s.config.RoomName, s.config.MaxUsers)

	s.wg.Add(1)
	go s.acceptConnections()

	return nil
}

func normalizeDNSName(name string) string {
	return strings.TrimSuffix(strings.TrimSpace(name), ".")
}

func listenerPort(listener net.Listener) (int, error) {
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return 0, err
	}
	return port, nil
}

// ConnectionAddress reports the authoritative endpoint after Start succeeds.
func (s *Server) ConnectionAddress() (string, int, bool) {
	s.endpointMu.RLock()
	defer s.endpointMu.RUnlock()
	if s.connectionHost == "" || s.connectionPort == 0 {
		return "", 0, false
	}
	return s.connectionHost, s.connectionPort, true
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

// Stop stops the chat server.
func (s *Server) Stop() error {
	s.stopOnce.Do(func() {
		s.stopErr = s.stop()
		close(s.stopDone)
	})
	<-s.stopDone
	return s.stopErr
}

func (s *Server) stop() error {
	log.Print("Stopping chat server...")
	s.cancel()

	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			log.Printf("Error closing listener: %v", err)
		}
	}

	s.mu.Lock()
	connections := make([]net.Conn, 0, len(s.connections))
	for _, conn := range s.connections {
		connections = append(connections, conn)
	}
	s.mu.Unlock()
	for _, conn := range connections {
		conn.Close()
	}

	if s.config.EnableTailscale && s.tsServer != nil {
		if err := s.tsServer.Close(); err != nil {
			log.Printf("Error closing Tailscale node: %v", err)
		}
	}

	handlersDone := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(handlersDone)
	}()

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	var stopErr error
	select {
	case <-handlersDone:
	case <-timer.C:
		stopErr = fmt.Errorf("timed out waiting for server goroutines to stop")
	}

	if s.chatRoom != nil {
		if err := s.chatRoom.Stop(); err != nil && stopErr == nil {
			stopErr = fmt.Errorf("stop chat room: %w", err)
		}
	}

	if stopErr != nil {
		return stopErr
	}
	log.Print("Chat server stopped")
	return nil
}
