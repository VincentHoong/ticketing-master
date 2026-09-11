// Command migrate applies or rolls back database schema migrations.
//
// Usage:
//
//	go run ./cmd/migrate up            # apply all pending migrations
//	go run ./cmd/migrate up 1          # apply the next 1 migration
//	go run ./cmd/migrate down 1        # roll back the last 1 migration
//	go run ./cmd/migrate down-all      # roll back every migration
//	go run ./cmd/migrate version       # print the current schema version
//	go run ./cmd/migrate force 3       # mark the db as being at version 3 without running it
//
// The migrations source directory can be overridden with MIGRATIONS_PATH
// (defaults to "migrations", relative to the backend module root).
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"ticketing-master/config"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("migrate: %v", err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate <up|down|down-all|version|force> [n]")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "migrations"
	}

	m, err := migrate.New("file://"+migrationsPath, toPgx5URL(cfg.PostgreSQLUrl))
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Printf("migrate: closing source: %v", srcErr)
		}
		if dbErr != nil {
			log.Printf("migrate: closing database: %v", dbErr)
		}
	}()

	command := args[0]
	switch command {
	case "up":
		if n, ok, err := optionalStep(args); err != nil {
			return err
		} else if ok {
			err = m.Steps(n)
		} else {
			err = m.Up()
		}
		return reportResult(err, "up")
	case "down":
		n, ok, err := optionalStep(args)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("usage: migrate down <n> (use down-all to roll back everything)")
		}
		return reportResult(m.Steps(-n), "down")
	case "down-all":
		return reportResult(m.Down(), "down-all")
	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			return reportResult(err, "version")
		}
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
		return nil
	case "force":
		if len(args) < 2 {
			return fmt.Errorf("usage: migrate force <version>")
		}
		version, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Errorf("invalid version %q: %w", args[1], err)
		}
		return m.Force(version)
	default:
		return fmt.Errorf("unknown command %q (want up|down|down-all|version|force)", command)
	}
}

func optionalStep(args []string) (n int, ok bool, err error) {
	if len(args) < 2 {
		return 0, false, nil
	}
	n, err = strconv.Atoi(args[1])
	if err != nil {
		return 0, false, fmt.Errorf("invalid step count %q: %w", args[1], err)
	}
	return n, true, nil
}

func reportResult(err error, command string) error {
	if err == nil {
		log.Printf("migrate %s: success", command)
		return nil
	}
	if errors.Is(err, migrate.ErrNoChange) {
		log.Printf("migrate %s: no change", command)
		return nil
	}
	return err
}

// toPgx5URL rewrites a standard postgres:// DSN to the pgx5:// scheme that
// golang-migrate's pgx/v5 database driver expects.
func toPgx5URL(postgresURL string) string {
	if idx := strings.Index(postgresURL, "://"); idx != -1 {
		return "pgx5" + postgresURL[idx:]
	}
	return "pgx5://" + postgresURL
}
