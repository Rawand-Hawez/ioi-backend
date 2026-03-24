package realtime

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// GlobalHub is the instance used by the application
var GlobalHub *Hub

// RegisterWSRoute sets up the WebSocket endpoint.
func RegisterWSRoute(app *fiber.App) {
	GlobalHub = NewHub()
	go GlobalHub.Run()
	go GlobalHub.StartPGListener()

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		// When a client connects
		GlobalHub.Register(c)

		defer func() {
			GlobalHub.Unregister(c)
		}()

		// Keep connection alive and listen for optional client messages
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				break
			}
			// Optional: Echo or handle client-to-server messages
			GlobalHub.Broadcast(string(message))
		}
	}))
}
