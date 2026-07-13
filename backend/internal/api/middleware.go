package api

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// jwtAuth verifies the Authorization: Bearer <token> header against
// cfg.JWTSecret. Applied to every protected route group.
func (s *Server) jwtAuth(c *fiber.Ctx) error {
	header := c.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return fiber.NewError(fiber.StatusUnauthorized, "missing bearer token")
	}
	tokenString := strings.TrimPrefix(header, "Bearer ")

	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(t *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
	}
	return c.Next()
}

// requestLogger logs every request's method, path, status, and duration.
func (s *Server) requestLogger(c *fiber.Ctx) error {
	start := time.Now()
	err := c.Next()
	s.log.Info("api: request",
		zap.String("method", c.Method()),
		zap.String("path", c.Path()),
		zap.Int("status", c.Response().StatusCode()),
		zap.Duration("duration", time.Since(start)),
	)
	return err
}
