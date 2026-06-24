// Command migrate applies SQL migrations from apps/tools/internal/db/migrations against
// the database referenced by DATABASE_URL.
// See: agent-context/lib/developer-guide.md
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/joho/godotenv"
	db "github.com/majtom2grndctrl/market-scout/apps/tools/internal/db"
)

func main() {
	if err := run(); err != nil {
		slog.Error("[migrate] migration failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load(".env.local") // no-op if absent; prod sets env vars directly

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}

	if len(os.Args) < 2 {
		return errors.New("missing verb argument; usage: migrate up|down|force|version")
	}

	verb := os.Args[1]
	switch verb {
	case "up", "down", "force", "version":
		// known verb
	default:
		return fmt.Errorf("unknown verb %q; expected up|down|force|version", verb)
	}

	// force parses and validates its argument before constructing the migrator,
	// so a bad argument never opens a DB connection.
	var forceVersion int
	if verb == "force" {
		if len(os.Args) < 3 {
			return errors.New("missing version argument; usage: migrate force <version>")
		}
		v, err := strconv.Atoi(os.Args[2])
		if err != nil || v < 0 {
			return fmt.Errorf("invalid force version %q; expected a non-negative integer", os.Args[2])
		}
		forceVersion = v
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

	switch verb {
	case "version":
		version, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			slog.Info("[migrate] no migrations applied")
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading migration version: %w", err)
		}
		slog.Info("[migrate] current version", "version", version, "dirty", dirty)
		return nil

	case "force":
		if err := m.Force(forceVersion); err != nil {
			return fmt.Errorf("forcing version %d: %w", forceVersion, err)
		}
		slog.Info("[migrate] forced version", "version", forceVersion)
		return nil

	case "up", "down":
		var runErr error
		if verb == "up" {
			runErr = m.Up()
		} else {
			runErr = m.Down()
		}
		if runErr != nil {
			if errors.Is(runErr, migrate.ErrNoChange) {
				slog.Info("[migrate] migrations up to date")
				return nil
			}
			return fmt.Errorf("migration %s failed: %w", verb, runErr)
		}
		slog.Info("[migrate] migration complete", "direction", verb)
		return nil
	}

	return nil
}
