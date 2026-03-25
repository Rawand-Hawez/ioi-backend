package realtime

import (
	"ioibackend/internal/config"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// RequireWSAuth validates a JWT from the ?token= query parameter before allowing
// a WebSocket upgrade. Must run before the websocket.New() handler.
func RequireWSAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenString := c.Query("token")
		if tokenString == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "WebSocket connections require ?token= query parameter",
			})
		}

		secret := []byte(config.Get().GoTrue.JWTSecret)
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fiber.ErrUnauthorized
			}
			return secret, nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token claims",
			})
		}

		if sub, ok := claims["sub"].(string); ok {
			c.Locals("ws_user_id", sub)
		}

		return c.Next()
	}
}
