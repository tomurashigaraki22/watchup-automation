package api

import (
	"github.com/gofiber/fiber/v2"

	"watchup/automation/internal/db/models"
)

// listCampaigns handles GET /api/v1/campaigns
func (s *Server) listCampaigns(c *fiber.Ctx) error {
	limit, offset := pagination(c)
	campaigns, err := s.repos.Campaigns.List(c.Context(), limit, offset)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to list campaigns")
	}
	return c.JSON(fiber.Map{"campaigns": campaigns})
}

type campaignRequest struct {
	Name       string `json:"name"`
	DailyLimit int    `json:"daily_limit"`
	SendMode   string `json:"send_mode"`
}

// createCampaign handles POST /api/v1/campaigns
func (s *Server) createCampaign(c *fiber.Ctx) error {
	var req campaignRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	if req.DailyLimit <= 0 {
		req.DailyLimit = 25
	}
	if req.SendMode != models.SendModeAutomatic {
		req.SendMode = models.SendModeManual
	}

	campaign := &models.OutreachCampaign{
		Name:       req.Name,
		Status:     models.CampaignStatusActive,
		DailyLimit: req.DailyLimit,
		SendMode:   req.SendMode,
	}
	if err := s.repos.Campaigns.Create(c.Context(), campaign); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to create campaign")
	}
	s.audit(c, "campaign.create", "campaign", campaign.ID, campaign.Name)
	return c.Status(fiber.StatusCreated).JSON(campaign)
}

type campaignUpdateRequest struct {
	Name       *string `json:"name"`
	Status     *string `json:"status"`
	DailyLimit *int    `json:"daily_limit"`
	SendMode   *string `json:"send_mode"`
}

// updateCampaign handles PATCH /api/v1/campaigns/:id — partial update.
func (s *Server) updateCampaign(c *fiber.Ctx) error {
	id, err := parseIDParam(c)
	if err != nil {
		return err
	}
	campaign, err := s.repos.Campaigns.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "campaign not found")
	}

	var req campaignUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Name != nil {
		campaign.Name = *req.Name
	}
	if req.Status != nil {
		campaign.Status = *req.Status
	}
	if req.DailyLimit != nil && *req.DailyLimit > 0 {
		campaign.DailyLimit = *req.DailyLimit
	}
	if req.SendMode != nil && (*req.SendMode == models.SendModeManual || *req.SendMode == models.SendModeAutomatic) {
		campaign.SendMode = *req.SendMode
	}

	if err := s.repos.Campaigns.Update(c.Context(), campaign); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update campaign")
	}
	s.audit(c, "campaign.update", "campaign", campaign.ID, campaign.Name)
	return c.JSON(campaign)
}

// deleteCampaign handles DELETE /api/v1/campaigns/:id — a soft delete
// (status=deleted) so historical emails/followups referencing it stay intact.
func (s *Server) deleteCampaign(c *fiber.Ctx) error {
	id, err := parseIDParam(c)
	if err != nil {
		return err
	}
	campaign, err := s.repos.Campaigns.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "campaign not found")
	}
	campaign.Status = models.CampaignStatusDeleted
	if err := s.repos.Campaigns.Update(c.Context(), campaign); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to delete campaign")
	}
	s.audit(c, "campaign.delete", "campaign", campaign.ID, campaign.Name)
	return c.JSON(campaign)
}

// pauseCampaign handles POST /api/v1/campaigns/:id/pause
func (s *Server) pauseCampaign(c *fiber.Ctx) error {
	return s.setCampaignStatus(c, models.CampaignStatusPaused, "campaign.pause")
}

// resumeCampaign handles POST /api/v1/campaigns/:id/resume
func (s *Server) resumeCampaign(c *fiber.Ctx) error {
	return s.setCampaignStatus(c, models.CampaignStatusActive, "campaign.resume")
}

func (s *Server) setCampaignStatus(c *fiber.Ctx, status, auditAction string) error {
	id, err := parseIDParam(c)
	if err != nil {
		return err
	}
	campaign, err := s.repos.Campaigns.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "campaign not found")
	}
	campaign.Status = status
	if err := s.repos.Campaigns.Update(c.Context(), campaign); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to update campaign status")
	}
	s.audit(c, auditAction, "campaign", campaign.ID, campaign.Name)
	return c.JSON(campaign)
}

// cloneCampaign handles POST /api/v1/campaigns/:id/clone — copies settings
// into a new campaign, left paused so it doesn't start sending unreviewed.
func (s *Server) cloneCampaign(c *fiber.Ctx) error {
	id, err := parseIDParam(c)
	if err != nil {
		return err
	}
	original, err := s.repos.Campaigns.GetByID(c.Context(), id)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "campaign not found")
	}

	clone := &models.OutreachCampaign{
		Name:       original.Name + " (Copy)",
		Status:     models.CampaignStatusPaused,
		DailyLimit: original.DailyLimit,
		SendMode:   original.SendMode,
	}
	if err := s.repos.Campaigns.Create(c.Context(), clone); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to clone campaign")
	}
	s.audit(c, "campaign.clone", "campaign", clone.ID, "cloned from "+original.Name)
	return c.Status(fiber.StatusCreated).JSON(clone)
}
