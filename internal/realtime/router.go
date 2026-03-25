package realtime

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// GlobalHub is the instance used by the application
var GlobalHub *Hub

// RegisterWSRoute sets up the authenticated WebSocket endpoint.
func RegisterWSRoute(app *fiber.App) {
	GlobalHub = NewHub()
	go GlobalHub.Run()
	go GlobalHub.StartPGListener()

	// JWT auth check runs before WebSocket upgrade
	app.Use("/ws", RequireWSAuth())

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		GlobalHub.Register(c)
		defer GlobalHub.Unregister(c)

		for {
			_, _, err := c.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
}
