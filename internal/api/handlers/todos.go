package handlers

import (
	"context"
	"fmt"
	"log"
	"os"

	"ioibackend/internal/db"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var Queries *db.Queries

// InitDB sets up the pgx connection pool mapped locally via Docker.
func InitDB() {
	dsn := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_PORT"),
		os.Getenv("POSTGRES_DB"),
	)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}

	Queries = db.New(pool)
	log.Println("PostgreSQL connection pool established via pgx")
}

// GetTodos returns the todos.
func GetTodos(c *fiber.Ctx) error {
	// Security: The user identity is securely extracted from the JWT token
	// which was verified mathematically by the RequireAuth middleware upstream.
	userIdLocal := c.Locals("user_id")
	userIdStr, ok := userIdLocal.(string)
	if !ok || userIdStr == "" {
		return c.Status(401).JSON(fiber.Map{"error": "Unauthorized context state"})
	}

	uid, err := uuid.Parse(userIdStr)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid JWT UUID claim"})
	}

	pgUUID := pgtype.UUID{Bytes: uid, Valid: true}
	todos, err := Queries.GetTodosForUser(c.Context(), pgUUID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": formatSqlError(err)})
	}

	// SQLC returns null if slice is completely empty, let's normalize to [] for Fiber JSON
	if todos == nil {
		todos = []db.Todo{}
	}

	return c.JSON(todos)
}

func formatSqlError(err error) string {
	return "Database Error: " + err.Error()
}
