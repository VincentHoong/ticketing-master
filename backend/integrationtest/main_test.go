//go:build integration

package integrationtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	testPostgresURL string
	testRedisURL    string
)

func TestMain(m *testing.M) {
	os.Exit(runTestMain(m))
}

func runTestMain(m *testing.M) int {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("ticketingmaster"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("secretpassword"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integrationtest: start postgres container: %v\n", err)
		return 1
	}
	defer func() {
		if err := testcontainers.TerminateContainer(pgContainer); err != nil {
			fmt.Fprintf(os.Stderr, "integrationtest: terminate postgres container: %v\n", err)
		}
	}()

	redisContainer, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integrationtest: start redis container: %v\n", err)
		return 1
	}
	defer func() {
		if err := testcontainers.TerminateContainer(redisContainer); err != nil {
			fmt.Fprintf(os.Stderr, "integrationtest: terminate redis container: %v\n", err)
		}
	}()

	pgDSN, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integrationtest: postgres connection string: %v\n", err)
		return 1
	}

	redisConnString, err := redisContainer.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integrationtest: redis connection string: %v\n", err)
		return 1
	}

	if err := runMigrations(pgDSN); err != nil {
		fmt.Fprintf(os.Stderr, "integrationtest: run migrations: %v\n", err)
		return 1
	}

	testPostgresURL = pgDSN
	testRedisURL = toRedisAddr(redisConnString)

	return m.Run()
}

func toRedisAddr(connString string) string {
	if idx := strings.Index(connString, "://"); idx != -1 {
		return connString[idx+3:]
	}
	return connString
}

func runMigrations(postgresURL string) error {
	migrationsPath, err := migrationsDir()
	if err != nil {
		return err
	}

	mig, err := migrate.New("file://"+migrationsPath, toPgx5URL(postgresURL))
	if err != nil {
		return err
	}
	defer mig.Close()

	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func migrationsDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to determine caller for migrations path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "migrations"), nil
}

func toPgx5URL(postgresURL string) string {
	if idx := strings.Index(postgresURL, "://"); idx != -1 {
		return "pgx5" + postgresURL[idx:]
	}
	return "pgx5://" + postgresURL
}
