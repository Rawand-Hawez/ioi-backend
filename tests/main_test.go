package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"testing"

	"IOI-real-estate-backend/internal/api/handlers"
	"IOI-real-estate-backend/internal/api/router"
	"IOI-real-estate-backend/internal/config"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

var testApp *fiber.App
var testToken string

func TestMain(m *testing.M) {
	// 1. Load configuration (will use env vars from Makefile or system)
	config.Load()

	// 2. Initialize database pool
	handlers.InitDB()

	// 3. Create Fiber app with routes + middleware
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		},
	})
	app.Use(logger.New())

	// 4. Register real routes
	router.SetupRoutes(app)
	testApp = app

	// 5. Get real GoTrue token
	token, err := authenticateTestUser()
	if err != nil {
		log.Fatalf("Failed to authenticate test user: %v", err)
	}
	testToken = token
	fmt.Println("Test infrastructure initialized successfully")

	// 6. Run tests
	os.Exit(m.Run())
}

func authenticateTestUser() (string, error) {
	authPayload, _ := json.Marshal(map[string]string{
		"email":    TestEmail,
		"password": TestPass,
	})

	// Try signup first (silent failure if exists)
	http.Post(GoTrueURL+"/signup", "application/json", bytes.NewBuffer(authPayload))

	// Login to get token
	resp, err := http.Post(GoTrueURL+"/token?grant_type=password", "application/json", bytes.NewBuffer(authPayload))
	if err != nil {
		return "", fmt.Errorf("failed to connect to GoTrue: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GoTrue login failed with status %d: %s", resp.StatusCode, string(body))
	}

	var auth AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		return "", fmt.Errorf("failed to decode auth response: %w", err)
	}

	return auth.AccessToken, nil
}
