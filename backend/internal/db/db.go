package db

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"watchup/automation/internal/config"
	"watchup/automation/internal/db/models"
)

// Connect opens a GORM Postgres connection using the provided config.
func Connect(cfg *config.Config) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(cfg.PostgresDSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	return gdb, nil
}

// Migrate runs AutoMigrate for all models.
func Migrate(gdb *gorm.DB) error {
	if err := gdb.AutoMigrate(models.All()...); err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}
	return nil
}
