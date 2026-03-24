package main

import (
	"log"
	"os"

	"ioibackend/internal/api/handlers"
	"ioibackend/internal/api/router"
	"ioibackend/internal/config"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
	// Load configuration
	config.Load()

	// Connect Database
	handlers.InitDB()

	// Initialize Fiber application
	app := fiber.New(fiber.Config{
		AppName:           "IOI Backend API",
		EnablePrintRoutes: true,
	})

	// Global Middleware
	app.Use(logger.New())

	// Health check route
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "up",
		})
	})

	// Setup custom routes
	router.SetupRoutes(app)

	// Start server
	port := os.Getenv("API_PORT_FIBER")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting Fiber API on :%s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatal(err)
	}
}
