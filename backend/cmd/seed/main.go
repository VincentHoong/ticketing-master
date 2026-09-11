// Command seed loads development/staging fixture data into the database.
//
// Usage:
//
//	go run ./cmd/seed
//
// Every file in the seed directory (default "seed", override with SEED_PATH)
// is executed in filename order, inside a single transaction. Seed files use
// fixed IDs and ON CONFLICT DO NOTHING, so re-running this command is safe.
//
// This is dev/staging tooling: seed files contain a shared, well-known
// password hash and must never be pointed at a production database.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"ticketing-master/config"
	"ticketing-master/repository"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("seed: %v", err)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	seedPath := os.Getenv("SEED_PATH")
	if seedPath == "" {
		seedPath = "seed"
	}

	files, err := seedFiles(seedPath)
	if err != nil {
		return fmt.Errorf("list seed files: %w", err)
	}
	if len(files) == 0 {
		log.Printf("seed: no .sql files found in %q, nothing to do", seedPath)
		return nil
	}

	repos, err := repository.NewRepositories(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer repos.Close()

	tx, err := repos.DbPool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", file, err)
		}

		if _, err := tx.Exec(ctx, string(contents)); err != nil {
			return fmt.Errorf("exec %s: %w", file, err)
		}
		log.Printf("seed: applied %s", filepath.Base(file))
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	log.Printf("seed: done (%d file(s))", len(files))
	return nil
}

func seedFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)

	return files, nil
}
