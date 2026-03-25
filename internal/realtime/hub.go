package realtime

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	clients   map[*websocket.Conn]string // conn → authenticated user_id
	broadcast chan []byte
	mu        sync.Mutex
}

// NewHub initializes a new WebSocket Hub.
func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*websocket.Conn]string),
		broadcast: make(chan []byte),
	}
}

// Register adds a new authenticated client to the hub.
func (h *Hub) Register(conn *websocket.Conn, userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = userID
	log.Printf("WebSocket client connected: user %s", userID)
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if userID, ok := h.clients[conn]; ok {
		delete(h.clients, conn)
		conn.Close()
		log.Printf("WebSocket client disconnected: user %s", userID)
	}
}

// Run starts the broadcast loop, routing each message to its owning user only.
// The payload is expected to be JSON with a "data.user_id" field (set by the
// notify_realtime() trigger). If no user_id is found, the message is broadcast
// to all connected clients.
func (h *Hub) Run() {
	for message := range h.broadcast {
		// Extract the target user_id from the pg_notify payload
		var envelope struct {
			Data struct {
				UserID string `json:"user_id"`
			} `json:"data"`
		}
		targetUserID := ""
		if err := json.Unmarshal(message, &envelope); err == nil {
			targetUserID = envelope.Data.UserID
		}

		// Snapshot clients under lock
		h.mu.Lock()
		type entry struct {
			conn   *websocket.Conn
			userID string
		}
		clients := make([]entry, 0, len(h.clients))
		for conn, uid := range h.clients {
			clients = append(clients, entry{conn, uid})
		}
		h.mu.Unlock()

		// Write outside the lock; skip clients that don't own this event
		for _, e := range clients {
			if targetUserID != "" && e.userID != targetUserID {
				continue
			}
			if err := e.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Error sending message to user %s: %v", e.userID, err)
				h.Unregister(e.conn)
			}
		}
	}
}

// Broadcast dispatches a raw message to all connected clients.
func (h *Hub) Broadcast(message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}
	h.broadcast <- data
}
