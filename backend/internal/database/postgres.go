// Package database opens the Postgres connection using GORM. Schema changes are
// managed by golang-migrate — GORM AutoMigrate is intentionally NOT called here
// (per spec section 22, migrations must be tracked & rollback-able).
package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/rajaku-printing/backend/internal/config"
)

func Open(ctx context.Context, cfg config.DBConfig, isDev bool) (*gorm.DB, error) {
	// gormLogLevel — deliberately logger.Warn in EVERY environment, including
	// development (isDev is no longer consulted here). logger.Info makes GORM
	// print every SQL statement WITH bound parameters to stdout — that used
	// to mean plaintext WhatsApp OTP codes (notification_jobs.message inserts,
	// phone_verifications writes) landed straight in application logs on any
	// dev machine, which defeats the point of hashing the code at rest
	// (review finding #3). Bump to logger.Info locally on your own machine
	// when you actually need to debug a specific query — never as the
	// checked-in default.
	gormLogLevel := logger.Warn

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger:      logger.Default.LogMode(gormLogLevel),
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("access underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return db, nil
}
