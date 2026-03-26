package middleware

import (
	"context"
	"time"

	"IOI-real-estate-backend/internal/db"
	"IOI-real-estate-backend/internal/db/pool"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func RequirePermission(permissionKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims := GetClaims(c)
		if claims == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "No JWT claims found",
			})
		}

		userIDStr, ok := claims["sub"].(string)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid user ID in claims",
			})
		}

		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid user ID format",
			})
		}

		p := pool.Get()
		if p == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Database pool not initialized",
			})
		}

		ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
		defer cancel()

		var hasPermission bool

		err = p.WithTx(ctx, claims, func(tx pgx.Tx) error {
			q := db.New(tx)
			hasPermission, err = q.CheckUserPermission(ctx, db.CheckUserPermissionParams{
				UserID: pgtype.UUID{Bytes: userID, Valid: true},
				Key:    permissionKey,
			})
			return err
		})

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to check permission",
			})
		}

		if !hasPermission {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Permission denied",
			})
		}

		return c.Next()
	}
}
