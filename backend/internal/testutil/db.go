// Package testutil provides shared test-only helpers for repository and
// seed tests. It is not imported by any production binary.
package testutil

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"watchup/automation/internal/db/models"
)

// NewDB returns an in-memory SQLite-backed GORM DB with all models migrated.
// Used in place of Postgres for fast repository unit tests where no live
// Postgres instance is available.
//
// Each test gets a uniquely-named in-memory database (keyed by t.Name()):
// "cache=shared" is required so GORM's connection pool sees one consistent
// schema within a test, but a fixed shared name would leak rows between
// tests running in the same process.
func NewDB(t *testing.T) *gorm.DB {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := "file:" + name + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("testutil: open sqlite: %v", err)
	}
	if err := db.AutoMigrate(models.All()...); err != nil {
		t.Fatalf("testutil: migrate: %v", err)
	}
	return db
}
