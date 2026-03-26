package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"IOI-real-estate-backend/internal/api/handlers"
	"IOI-real-estate-backend/internal/api/router"
	"IOI-real-estate-backend/internal/cache"
	"IOI-real-estate-backend/internal/config"
	"IOI-real-estate-backend/internal/db/pool"
	"IOI-real-estate-backend/internal/realtime"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
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

	// Setup Realtime WebSockets (JWT-authenticated)
	realtime.RegisterWSRoute(app)

	// Global Middleware
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(logger.New())
	app.Use(helmet.New())
	app.Use(compress.New())
	app.Use(limiter.New(limiter.Config{
		Max:               100,
		Expiration:        1 * time.Minute,
		LimiterMiddleware: limiter.SlidingWindow{},
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: cfg.API.CORSOrigins,
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PATCH, DELETE, OPTIONS",
	}))

	// Health check route — pings real dependencies
	app.Get("/health", func(c *fiber.Ctx) error {
		pgStatus := "disconnected"
		cacheStatus := "disconnected"

		if p := pool.Get(); p != nil {
			ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
			defer cancel()
			if err := p.Ping(ctx); err == nil {
				pgStatus = "connected"
			}
		}

		if cache.Client != nil {
			ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
			defer cancel()
			if err := cache.Client.Ping(ctx).Err(); err == nil {
				cacheStatus = "connected"
			}
		}

		status := fiber.StatusOK
		overall := "up"
		if pgStatus != "connected" {
			status = fiber.StatusServiceUnavailable
			overall = "degraded"
		}

		return c.Status(status).JSON(fiber.Map{
			"status": overall,
			"services": fiber.Map{
				"postgres": pgStatus,
				"cache":    cacheStatus,
			},
		})
	})

	// Setup custom routes
	router.SetupRoutes(app)

	// Start server in goroutine
	port := cfg.API.FiberPort
	if port == "" {
		port = "8080"
	}

	go func() {
		log.Printf("Starting Fiber API on :%s", port)
		if err := app.Listen(":" + port); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("Forced shutdown: %v", err)
	}

	if p := pool.Get(); p != nil {
		p.Close()
	}

	if cache.Client != nil {
		cache.Client.Close()
	}

	log.Println("Shutdown complete")
}
