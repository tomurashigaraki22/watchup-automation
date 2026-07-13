package seed

import (
	"context"

	"gorm.io/gorm"

	"watchup/automation/internal/db/models"
)

// Default ensures at least one outreach campaign exists so the pipeline has
// somewhere to attach discovered companies and generated emails. It is a
// no-op if any campaign already exists.
func Default(ctx context.Context, db *gorm.DB) error {
	var count int64
	if err := db.WithContext(ctx).Model(&models.OutreachCampaign{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	campaign := &models.OutreachCampaign{
		Name:       "Default Partnership Outreach",
		Status:     models.CampaignStatusActive,
		DailyLimit: 25,
		SendMode:   models.SendModeManual,
	}
	return db.WithContext(ctx).Create(campaign).Error
}
