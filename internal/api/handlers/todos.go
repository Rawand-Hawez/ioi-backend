package handlers

import (
	"log"

	"ioibackend/internal/api/middleware"
	"ioibackend/internal/config"
	"ioibackend/internal/db"
	"ioibackend/internal/db/pool"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// InitDB sets up the pgx connection pool
func InitDB() {
	cfg := config.Get()
	if cfg == nil {
		log.Fatal("Configuration not loaded")
	}

	dsn := cfg.GetPostgresDSN()
	if err := pool.Init(dsn); err != nil {
		log.Fatalf("Unable to initialize connection pool: %v", err)
	}

	log.Println("PostgreSQL connection pool established via pgx with GUC support")
}

// GetTodosRLS returns todos using RLS (GUC injection required)
func GetTodosRLS(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	var todos []db.Todo

	err := p.WithTx(c.Context(), claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		todos, err = q.GetTodosRLS(c.Context())
		return err
	})

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": formatSqlError(err)})
	}

	if todos == nil {
		todos = []db.Todo{}
	}

	return c.JSON(todos)
}

// CreateTodoRLS creates a todo using RLS (GUC injection required)
func CreateTodoRLS(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	type CreateTodoRequest struct {
		Task string `json:"task"`
	}

	var req CreateTodoRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if req.Task == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Task is required"})
	}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	var todo db.Todo

	err := p.WithTx(c.Context(), claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		todo, err = q.CreateTodoRLS(c.Context(), req.Task)
		return err
	})

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": formatSqlError(err)})
	}

	return c.Status(201).JSON(todo)
}

// ToggleTodoRLS toggles a todo's completion status using RLS
func ToggleTodoRLS(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	todoID := c.Params("id")
	if todoID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Todo ID is required"})
	}

	uid, err := uuid.Parse(todoID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid todo ID format"})
	}

	pgUUID := pgtype.UUID{Bytes: uid, Valid: true}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	err = p.WithTx(c.Context(), claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		return q.ToggleTodoRLS(c.Context(), pgUUID)
	})

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": formatSqlError(err)})
	}

	return c.JSON(fiber.Map{"message": "Todo toggled successfully"})
}

// DeleteTodoRLS deletes a todo using RLS
func DeleteTodoRLS(c *fiber.Ctx) error {
	claims := middleware.GetClaims(c)
	if claims == nil {
		return c.Status(401).JSON(fiber.Map{"error": "No JWT claims found"})
	}

	todoID := c.Params("id")
	if todoID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Todo ID is required"})
	}

	uid, err := uuid.Parse(todoID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid todo ID format"})
	}

	pgUUID := pgtype.UUID{Bytes: uid, Valid: true}

	p := pool.Get()
	if p == nil {
		return c.Status(500).JSON(fiber.Map{"error": "Database pool not initialized"})
	}

	err = p.WithTx(c.Context(), claims, func(tx pgx.Tx) error {
		q := db.New(tx)
		return q.DeleteTodoRLS(c.Context(), pgUUID)
	})

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": formatSqlError(err)})
	}

	return c.JSON(fiber.Map{"message": "Todo deleted successfully"})
}

func formatSqlError(err error) string {
	return "Database Error: " + err.Error()
}
