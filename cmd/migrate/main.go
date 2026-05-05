// Command migrate applies SQL migrations from internal/db/migrations against
// the database referenced by DATABASE_URL.
// See: agent-context/lib/developer-guide.md
package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const migrationsSource = "file://internal/db/migrations"

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("[migrate] DATABASE_URL is not set")
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		slog.Error("[migrate] missing direction argument", "usage", "migrate up|down")
		os.Exit(1)
	}

	direction := os.Args[1]
	if direction != "up" && direction != "down" {
		slog.Error("[migrate] unknown direction", "direction", direction, "expected", "up|down")
		os.Exit(1)
	}

	m, err := migrate.New(migrationsSource, dbURL)
	if err != nil {
		slog.Error("[migrate] failed to initialize migrator", "err", err)
		os.Exit(1)
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
			return
		}
		slog.Error("[migrate] migration failed", "direction", direction, "err", runErr)
		os.Exit(1)
	}

	slog.Info("[migrate] migration complete", "direction", direction)
}
