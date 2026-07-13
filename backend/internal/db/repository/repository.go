package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Repository is a generic create/get/list/update wrapper. Companies,
// contacts, campaigns, emails, followups, ai_generations, suppressions, and
// audit_logs all share this exact shape, so one generic implementation
// replaces eight near-identical hand-written repos.
type Repository[T any] struct {
	db *gorm.DB
}

// New builds a Repository for entity type T.
func New[T any](db *gorm.DB) *Repository[T] {
	return &Repository[T]{db: db}
}

// Create inserts entity and populates its generated fields (ID, timestamps).
func (r *Repository[T]) Create(ctx context.Context, entity *T) error {
	return r.db.WithContext(ctx).Create(entity).Error
}

// GetByID fetches a single entity by primary key.
func (r *Repository[T]) GetByID(ctx context.Context, id uint) (*T, error) {
	var entity T
	if err := r.db.WithContext(ctx).First(&entity, id).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

// List returns entities newest-first. limit/offset <= 0 mean "unbounded"/"from the start".
func (r *Repository[T]) List(ctx context.Context, limit, offset int) ([]T, error) {
	var entities []T
	q := r.db.WithContext(ctx).Order("id desc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Find(&entities).Error; err != nil {
		return nil, err
	}
	return entities, nil
}

// Update persists all fields of entity (it must already have a primary key).
func (r *Repository[T]) Update(ctx context.Context, entity *T) error {
	return r.db.WithContext(ctx).Save(entity).Error
}

// First returns the first entity matching the given GORM query condition, or
// ok=false if none exists. Used for unique-field lookups (dedupe by
// website, contact by email, suppression by email, ...) without a one-off
// method per entity.
func (r *Repository[T]) First(ctx context.Context, query string, args ...any) (*T, bool, error) {
	var entity T
	err := r.db.WithContext(ctx).Where(query, args...).First(&entity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &entity, true, nil
}

// Count returns the number of rows matching query (e.g. daily send-limit
// checks). Pass an empty query to count every row.
func (r *Repository[T]) Count(ctx context.Context, query string, args ...any) (int64, error) {
	var count int64
	q := r.db.WithContext(ctx).Model(new(T))
	if query != "" {
		q = q.Where(query, args...)
	}
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ListWhere is List filtered by a GORM query condition — used for
// status-based pipeline sweeps (e.g. companies still at status=discovered).
func (r *Repository[T]) ListWhere(ctx context.Context, limit, offset int, query string, args ...any) ([]T, error) {
	var entities []T
	q := r.db.WithContext(ctx).Where(query, args...).Order("id desc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Find(&entities).Error; err != nil {
		return nil, err
	}
	return entities, nil
}
