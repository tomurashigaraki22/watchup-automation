package validation

import (
	"context"

	"go.uber.org/zap"

	"watchup/automation/internal/db/models"
	"watchup/automation/internal/db/repository"
)

// Service validates contacts and persists verification results.
type Service struct {
	repo      *repository.Repositories
	validator *Validator
	log       *zap.Logger
}

// NewService builds a validation Service.
func NewService(repo *repository.Repositories, validator *Validator, log *zap.Logger) *Service {
	return &Service{repo: repo, validator: validator, log: log}
}

// ValidateContact scores contact.Email and persists verified/verification_score.
func (s *Service) ValidateContact(ctx context.Context, contact *models.Contact) (Result, error) {
	result, err := s.validator.Validate(ctx, contact.Email)
	if err != nil {
		return Result{}, err
	}

	contact.Verified = result.Valid
	contact.VerificationScore = result.Score
	if err := s.repo.Contacts.Update(ctx, contact); err != nil {
		return result, err
	}

	s.log.Info("validation: contact scored",
		zap.Uint("contact_id", contact.ID), zap.String("email", contact.Email),
		zap.Int("score", result.Score), zap.Bool("valid", result.Valid))
	return result, nil
}
