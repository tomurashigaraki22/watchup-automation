package repository

import (
	"gorm.io/gorm"

	"watchup/automation/internal/db/models"
)

// Repositories aggregates one generic Repository per entity for convenient
// dependency injection into the API, workers, and scheduler.
type Repositories struct {
	Companies     *Repository[models.Company]
	Contacts      *Repository[models.Contact]
	Campaigns     *Repository[models.OutreachCampaign]
	Emails        *Repository[models.Email]
	Followups     *Repository[models.Followup]
	AIGenerations *Repository[models.AIGeneration]
	Suppressions  *Repository[models.Suppression]
	AuditLogs     *Repository[models.AuditLog]
}

// NewRepositories builds the full set of repositories over a single DB handle.
func NewRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		Companies:     New[models.Company](db),
		Contacts:      New[models.Contact](db),
		Campaigns:     New[models.OutreachCampaign](db),
		Emails:        New[models.Email](db),
		Followups:     New[models.Followup](db),
		AIGenerations: New[models.AIGeneration](db),
		Suppressions:  New[models.Suppression](db),
		AuditLogs:     New[models.AuditLog](db),
	}
}
