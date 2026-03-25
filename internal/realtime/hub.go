package realtime

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	// Registered clients.
	clients map[*websocket.Conn]bool
	// Inbound messages from the clients or the Postgres listener.
	broadcast chan []byte
	// Lock for client map operations
	mu sync.Mutex
}

// NewHub initializes a new WebSocket Hub.
func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan []byte),
	}
}

// Register adds a new client to the hub.
func (h *Hub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = true
	log.Printf("New WebSocket client registered: %p", conn)
}

// Unregister removes a client from the hub.
func (h *Hub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[conn]; ok {
		delete(h.clients, conn)
		conn.Close()
		log.Printf("WebSocket client disconnected: %p", conn)
	}
}

// Run starts the broadcast loop.
func (h *Hub) Run() {
	for message := range h.broadcast {
		// Snapshot clients under lock
		h.mu.Lock()
		clients := make([]*websocket.Conn, 0, len(h.clients))
		for client := range h.clients {
			clients = append(clients, client)
		}
		h.mu.Unlock()

		// Write outside the lock so one slow client cannot stall all broadcasts
		for _, client := range clients {
			if err := client.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("Error sending message to client %p: %v", client, err)
				h.Unregister(client)
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
