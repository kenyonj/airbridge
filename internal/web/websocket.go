package web

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local use
	},
}

// WebSocketMessage represents a message sent over WebSocket.
type WebSocketMessage struct {
	Type           string `json:"type"` // "state_update", "renderer_changed", etc.
	RendererID     string `json:"renderer_id,omitempty"`
	TransportState string `json:"transport_state,omitempty"`
	Running        bool   `json:"running,omitempty"`
	Action         string `json:"action,omitempty"` // "created", "deleted", "updated"
}

// StateUpdate is an alias for backward compatibility.
type StateUpdate = WebSocketMessage

// Hub maintains active WebSocket connections and broadcasts updates.
type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan WebSocketMessage
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan WebSocketMessage, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

// Run starts the hub's main loop.
func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.clients[conn] = true
			h.mu.Unlock()
			log.Printf("WebSocket client connected (%d total)", len(h.clients))

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
			h.mu.Unlock()
			log.Printf("WebSocket client disconnected (%d total)", len(h.clients))

		case update := <-h.broadcast:
			h.mu.RLock()
			data, _ := json.Marshal(update)
			for conn := range h.clients {
				err := conn.WriteMessage(websocket.TextMessage, data)
				if err != nil {
					conn.Close()
					h.mu.RUnlock()
					h.mu.Lock()
					delete(h.clients, conn)
					h.mu.Unlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a state update to all connected clients.
func (h *Hub) Broadcast(update StateUpdate) {
	select {
	case h.broadcast <- update:
	default:
		// Channel full, drop the message
		log.Println("WebSocket broadcast channel full, dropping message")
	}
}

// HandleWebSocket upgrades HTTP connections to WebSocket.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	h.register <- conn

	// Read loop (handles pings and detects disconnection)
	go func() {
		defer func() {
			h.unregister <- conn
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}
