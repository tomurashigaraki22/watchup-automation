package api

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

const tokenTTL = 24 * time.Hour

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// login handles POST /api/v1/auth/login. This build has one admin user
// (no user table in the PRD schema) configured via ADMIN_USERNAME/ADMIN_PASSWORD.
func (s *Server) login(c *fiber.Ctx) error {
	if s.cfg.AdminPassword == "" {
		return fiber.NewError(fiber.StatusServiceUnavailable, "dashboard login is not configured (ADMIN_PASSWORD unset)")
	}

	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Username != s.cfg.AdminUsername || req.Password != s.cfg.AdminPassword {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}

	expiresAt := time.Now().Add(tokenTTL)
	claims := jwt.RegisteredClaims{
		Subject:   req.Username,
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to sign token")
	}

	return c.JSON(loginResponse{Token: signed, ExpiresAt: expiresAt})
}
