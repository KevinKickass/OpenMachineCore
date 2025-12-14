package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/KevinKickass/OpenMachineCore/internal/auth"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 8192

	// Send channel buffer size
	sendBufferSize = 256
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Implement proper origin checking for production
		return true
	},
}

// Client represents a WebSocket client connection
type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []byte
	logger        *zap.Logger
	authenticated bool
	permissions   []auth.Permission
	userID        *uuid.UUID
}

// readPump handles reading messages from the WebSocket connection
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)

	// 10 seconds timeout for authentication
	c.conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	for {
		var msg map[string]interface{}
		if err := c.conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseAbnormalClosure) {
				c.logger.Warn("WebSocket read error",
					zap.Error(err),
					zap.String("remote_addr", c.conn.RemoteAddr().String()))
			}
			break
		}

		// First message MUST be authentication
		if !c.authenticated {
			if msgType, ok := msg["type"].(string); !ok || msgType != "auth" {
				c.sendAuthFailed("First message must be authentication")
				c.conn.Close()
				return
			}

			token, ok := msg["token"].(string)
			if !ok || token == "" {
				c.sendAuthFailed("Missing token in auth message")
				c.conn.Close()
				return
			}

			// Validate token via AuthService
			authService := c.hub.authService
			permissions, err := authService.ValidateToken(
				context.Background(),
				token,
				c.conn.RemoteAddr().String(),
				"", // User-Agent not available in WebSocket
			)

			if err != nil {
				c.logger.Warn("WebSocket authentication failed",
					zap.Error(err),
					zap.String("remote_addr", c.conn.RemoteAddr().String()))
				c.sendAuthFailed("Invalid or expired token")
				c.conn.Close()
				return
			}

			// Authentication successful
			c.authenticated = true
			c.permissions = permissions
			c.conn.SetReadDeadline(time.Time{}) // Remove deadline

			c.sendAuthSuccess(permissions)
			c.logger.Info("WebSocket client authenticated",
				zap.String("remote_addr", c.conn.RemoteAddr().String()),
				zap.Any("permissions", permissions))

			// NOW register to hub (only after auth)
			c.hub.register <- c
			continue
		}

		// Handle other client messages (subscriptions, etc.)
		c.handleMessage(msg)
	}
}

func (c *Client) sendAuthSuccess(permissions []auth.Permission) {
	msg := map[string]interface{}{
		"type":        "auth_success",
		"timestamp":   time.Now(),
		"permissions": permissions,
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}

func (c *Client) sendAuthFailed(reason string) {
	msg := map[string]interface{}{
		"type":      "auth_failed",
		"timestamp": time.Now(),
		"reason":    reason,
	}
	data, _ := json.Marshal(msg)
	c.send <- data
}

func (c *Client) handleMessage(msg map[string]interface{}) {
	// Handle client commands (e.g., subscribe to specific devices)
	c.logger.Debug("Received client message",
		zap.String("remote_addr", c.conn.RemoteAddr().String()),
		zap.Any("message", msg))

	// TODO: Implement subscription logic
}

// writePump handles writing messages to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Coalesce queued messages into current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ServeWs handles WebSocket upgrade requests
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		hub.logger.Error("WebSocket upgrade error",
			zap.Error(err),
			zap.String("remote_addr", r.RemoteAddr))
		return
	}

	client := &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, sendBufferSize),
		logger: hub.logger, // <- Logger vom Hub übernehmen
	}

	client.hub.register <- client

	// Start read and write pumps in separate goroutines
	go client.writePump()
	go client.readPump()
}
