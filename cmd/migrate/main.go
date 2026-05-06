// Command migrate applies SQL migrations from internal/db/migrations against
// the database referenced by DATABASE_URL.
// See: agent-context/lib/developer-guide.md
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	db "github.com/majtom2grndctrl/market-scout/internal/db"
)

func main() {
	if err := run(); err != nil {
		slog.Error("[migrate] " + err.Error())
		os.Exit(1)
	}
}

func run() error {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}

	if len(os.Args) < 2 {
		return errors.New("missing direction argument; usage: migrate up|down")
	}

	direction := os.Args[1]
	if direction != "up" && direction != "down" {
		return fmt.Errorf("unknown direction %q; expected up|down", direction)
	}

	src, err := iofs.New(db.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("failed to open embedded migrations: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dbURL)
	if err != nil {
		return fmt.Errorf("failed to initialize migrator: %w", err)
	}
	defer func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			slog.Warn("[migrate] error closing migrator", "source_err", srcErr, "db_err", dbErr)
		}
	}()

	var runErr error
	switch direction {
	case "up":
		runErr = m.Up()
	case "down":
		runErr = m.Down()
	}

	if runErr != nil {
		if errors.Is(runErr, migrate.ErrNoChange) {
			slog.Info("[migrate] migrations up to date")
			return nil
		}
		return fmt.Errorf("migration %s failed: %w", direction, runErr)
	}

	slog.Info("[migrate] migration complete", "direction", direction)
	return nil
}
