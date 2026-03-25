package middleware

import (
	"strings"

	"ioibackend/internal/config"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// RequireAuth intercepts the request, verifies the JWT using the Gotrue secret,
// and injects the extracted user claims into the Fiber Context.
func RequireAuth() fiber.Handler {
	secret := []byte(config.Get().GoTrue.JWTSecret)

	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing or improperly formatted Authorization Bearer token",
			})
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		// Parse and mathematically verify the JWT signature mechanism
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.ErrUnauthorized
			}
			return secret, nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid, expired, or improperly signed JWT token",
			})
		}

		// Extract JSON payload
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Cannot parse JWT payload claims",
			})
		}

		// Pass the exact Identity Context downwards to the handlers securely
		c.Locals("user_id", claims["sub"])
		c.Locals("role", claims["role"])

		return c.Next()
	}
}
