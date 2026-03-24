// Package database provides PostgreSQL connectivity via GORM and schema
// migration support via golang-migrate.
package database

import (
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxLifetime = 1 * time.Hour
)

// Connect opens a PostgreSQL connection using GORM and configures connection
// pooling. It returns the configured *gorm.DB instance or an error if the
// connection cannot be established.
func Connect(databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("getting underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	slog.Info("database connected")
	return db, nil
}
