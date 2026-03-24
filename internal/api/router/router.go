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

	// Public demo endpoint
	api.Get("/demo", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Custom Fiber JSON response from IOI Backend API",
		})
	})

	// Authenticated routes with RLS (GUC injection for Row-Level Security)
	todos := api.Group("/todos", middleware.RequireAuth(), middleware.InjectGUCVariables())
	todos.Get("/", handlers.GetTodosRLS)
	todos.Post("/", handlers.CreateTodoRLS)
	todos.Patch("/:id/toggle", handlers.ToggleTodoRLS)
	todos.Delete("/:id", handlers.DeleteTodoRLS)
}
