package database

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// RunMigrations applies all pending database migrations from the migrations/
// directory. It returns nil if all migrations succeed or if there are no new
// migrations to apply. If the migrations directory does not exist, it is
// skipped gracefully.
func RunMigrations(databaseURL string) error {
	if _, err := os.Stat("migrations"); os.IsNotExist(err) {
		slog.Info("no migrations directory, skipping")
		return nil
	}

	m, err := migrate.New("file://migrations", databaseURL)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			slog.Warn("failed to close migration source", "error", srcErr)
		}
		if dbErr != nil {
			slog.Warn("failed to close migration database", "error", dbErr)
		}
	}()

	err = m.Up()
	if errors.Is(err, migrate.ErrNoChange) {
		slog.Info("no new migrations to apply")
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("running migrations: %w", err)
	}

	if err == nil {
		slog.Info("migrations applied successfully")
	}
	return nil
}
