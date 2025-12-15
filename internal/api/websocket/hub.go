package websocket

import (
	"encoding/json"
	"sync"

	"github.com/KevinKickass/OpenMachineCore/internal/auth"
	"go.uber.org/zap"
)

// MachineStatusProvider interface for getting current machine status
type MachineStatusProvider interface {
	GetStatus() any
}

// Hub maintains active WebSocket clients and broadcasts messages
type Hub struct {
	// Registered clients
	clients map[*Client]bool

	// Inbound messages to broadcast
	broadcast chan Message

	// Register requests from clients
	register chan *Client

	// Unregister requests from clients
	unregister chan *Client

	// Mutex for thread-safe operations
	mu sync.RWMutex

	// Logger
	logger *zap.Logger

	//Auth Service
	authService *auth.AuthService

	// Machine status provider (optional)
	machineStatusProvider MachineStatusProvider
}

// NewHub creates a new Hub instance
func NewHub(logger *zap.Logger, authService *auth.AuthService) *Hub {
	return &Hub{
		broadcast:   make(chan Message, 256),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		clients:     make(map[*Client]bool),
		logger:      logger,
		authService: authService,
	}
}

// SetMachineStatusProvider sets the machine status provider
func (h *Hub) SetMachineStatusProvider(provider MachineStatusProvider) {
	h.machineStatusProvider = provider
}

// Run starts the hub's main event loop
func (h *Hub) Run() {
	h.logger.Info("WebSocket Hub started")
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Info("WebSocket client registered",
				zap.String("remote_addr", client.conn.RemoteAddr().String()),
				zap.Int("total_clients", len(h.clients)))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.logger.Info("WebSocket client unregistered",
					zap.String("remote_addr", client.conn.RemoteAddr().String()),
					zap.Int("total_clients", len(h.clients)))
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			data, err := json.Marshal(message)
			if err != nil {
				h.logger.Error("Failed to marshal broadcast message",
					zap.Error(err))
				h.mu.RUnlock()
				continue
			}

			for client := range h.clients {
				select {
				case client.send <- data:
					// Message sent successfully
				default:
					// Client send channel full - unregister slow/dead client
					close(client.send)
					delete(h.clients, client)
					h.logger.Warn("Client send buffer full, unregistering",
						zap.String("remote_addr", client.conn.RemoteAddr().String()))
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(msg Message) {
	select {
	case h.broadcast <- msg:
		// Message queued for broadcast
	default:
		h.logger.Warn("Hub broadcast channel full, message dropped",
			zap.String("message_type", string(msg.Type)))
	}
}

// GetClientCount returns the number of connected clients
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
