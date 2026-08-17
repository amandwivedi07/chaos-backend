// Package database owns the GORM connection and connection-pool tuning.
// Repositories receive *gorm.DB via constructors; nothing else touches it.
package database

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/chaosapp/backend/internal/config"
)

// Connect opens PostgreSQL with pooling. Auto-migration is intentionally
// DISABLED — schema changes go through migration files only (see Migrate).
func Connect(cfg config.DatabaseConfig, log *zap.Logger) (*gorm.DB, error) {
	level := gormlogger.Warn
	if cfg.LogQueries {
		level = gormlogger.Info
	}

	db, err := gorm.Open(postgres.Open(cfg.URL), &gorm.Config{
		Logger:  gormlogger.Default.LogMode(level),
		NowFunc: func() time.Time { return time.Now().UTC() },
		// PrepareStmt caches prepared statements — parameterized everywhere.
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("unwrap sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	log.Info("postgres connected",
		zap.Int("max_open_conns", cfg.MaxOpenConns))
	return db, nil
}
