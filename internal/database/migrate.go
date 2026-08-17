package database

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // driver
	_ "github.com/golang-migrate/migrate/v4/source/file"       // file:// source
	"go.uber.org/zap"
)

// Migrate applies pending SQL migrations from dir (file://migrations).
// golang-migrate tracks state in the schema_migrations table.
func Migrate(databaseURL, dir string, log *zap.Logger) error {
	m, err := migrate.New("file://"+dir, databaseURL)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info("migrations: up to date")
			return nil
		}
		return fmt.Errorf("apply migrations: %w", err)
	}
	log.Info("migrations: applied")
	return nil
}
