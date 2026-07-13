package api

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"watchup/automation/internal/db/models"
)

// parseIDParam reads the ":id" route param as a positive uint.
func parseIDParam(c *fiber.Ctx) (uint, error) {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid id")
	}
	return uint(id), nil
}

// pagination reads ?limit=&offset= query params with sane defaults/caps.
func pagination(c *fiber.Ctx) (limit, offset int) {
	limit = c.QueryInt("limit", 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset = c.QueryInt("offset", 0)
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// audit records an API-triggered mutation. Failures are logged, not
// propagated — an audit-log write failure shouldn't fail the request.
func (s *Server) audit(c *fiber.Ctx, action, entity string, entityID uint, detail string) {
	entry := &models.AuditLog{
		Actor:    "api",
		Action:   action,
		Entity:   entity,
		EntityID: entityID,
		Detail:   detail,
	}
	if err := s.repos.AuditLogs.Create(c.Context(), entry); err != nil {
		s.log.Error("api: failed to write audit log")
	}
}
