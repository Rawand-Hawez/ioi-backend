package main

import (
	"log"

	"ioibackend/internal/api/handlers"
	"ioibackend/internal/api/router"
	"ioibackend/internal/cache"
	"ioibackend/internal/config"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	// Load configuration with validation
	config.Load()

	// Get configuration
	cfg := config.Get()

	// Connect Database with GUC support
	handlers.InitDB()

	// Initialize Cache (Dragonfly)
	cache.InitCache()

	// Initialize Fiber application
	app := fiber.New(fiber.Config{
		AppName:           "IOI Backend API",
		EnablePrintRoutes: true,
	})

	// Setup Realtime WebSockets
	// realtime.RegisterWSRoute(app)

	// Global Middleware
	app.Use(recover.New())
	app.Use(logger.New())

	// Health check route
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "up",
			"services": fiber.Map{
				"postgres": "connected",
				"cache":    "connected",
			},
		})
	})

	// Setup custom routes
	router.SetupRoutes(app)

	// Start server
	port := cfg.API.FiberPort
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Fiber API on :%s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatal(err)
	}
}
