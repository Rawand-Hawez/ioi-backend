package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// GUCContextKey is the key used to store JWT claims in Fiber locals
const GUCContextKey = "jwt_claims"

// InjectGUCVariables creates a middleware that injects JWT claims as PostgreSQL GUC variables
// This middleware must be used AFTER RequireAuth middleware
// It stores the claims in context so handlers can use pool.WithTx with GUC injection
func InjectGUCVariables() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get user_id and role from Locals (set by RequireAuth middleware)
		userID := c.Locals("user_id")
		role := c.Locals("role")

		// Build claims map for GUC injection
		claims := make(map[string]interface{})

		if userID != nil {
			if uid, ok := userID.(string); ok {
				claims["sub"] = uid
			}
		}

		if role != nil {
			if r, ok := role.(string); ok && r != "" {
				claims["role"] = r
			}
		}

		// Store claims in Locals for handlers to use
		c.Locals(GUCContextKey, claims)

		return c.Next()
	}
}

// GetClaims extracts JWT claims from Fiber context
func GetClaims(c *fiber.Ctx) map[string]interface{} {
	claims, ok := c.Locals(GUCContextKey).(map[string]interface{})
	if !ok {
		return nil
	}
	return claims
}

