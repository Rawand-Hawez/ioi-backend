package router

import (
	"ioibackend/internal/api/handlers"
	"ioibackend/internal/api/middleware"

	"github.com/gofiber/fiber/v2"
)

// SetupRoutes organizes all the custom API endpoints for the Go backend.
func SetupRoutes(app *fiber.App) {
	// API Group
	api := app.Group("/api/v1")

	api.Get("/demo", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Custom Fiber JSON response securely proxied past pREST",
		})
	})

	// Setup custom resource
	api.Use("/todos", middleware.RequireAuth())
	api.Get("/todos", handlers.GetTodos)
}
