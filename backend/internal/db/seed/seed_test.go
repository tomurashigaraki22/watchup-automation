package seed_test

import (
	"context"
	"testing"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/seed"
	"watchup/automation/internal/testutil"
)

func TestDefault_CreatesOneCampaignWhenNoneExist(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewDB(t)

	if err := seed.Default(ctx, db); err != nil {
		t.Fatalf("seed default: %v", err)
	}

	var campaigns []models.OutreachCampaign
	if err := db.Find(&campaigns).Error; err != nil {
		t.Fatalf("list campaigns: %v", err)
	}
	if len(campaigns) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(campaigns))
	}
	if campaigns[0].Status != models.CampaignStatusActive || campaigns[0].SendMode != models.SendModeManual {
		t.Fatalf("unexpected default campaign: %+v", campaigns[0])
	}
}

func TestDefault_NoOpWhenCampaignExists(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewDB(t)

	if err := db.Create(&models.OutreachCampaign{Name: "Existing", Status: models.CampaignStatusActive, DailyLimit: 10}).Error; err != nil {
		t.Fatalf("seed existing campaign: %v", err)
	}

	if err := seed.Default(ctx, db); err != nil {
		t.Fatalf("seed default: %v", err)
	}

	var count int64
	if err := db.Model(&models.OutreachCampaign{}).Count(&count).Error; err != nil {
		t.Fatalf("count campaigns: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected seed to be a no-op, got %d campaigns", count)
	}
}
