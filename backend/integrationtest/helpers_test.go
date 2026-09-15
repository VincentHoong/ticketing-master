//go:build integration

package integrationtest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"ticketing-master/config"
	"ticketing-master/repository"
	"ticketing-master/repository/events"

	"github.com/google/uuid"
)

func newTestRepositories(t *testing.T, overrides ...func(*config.Config)) *repository.Repositories {
	t.Helper()

	cfg := &config.Config{
		JwtSecret:          "test-secret",
		PostgreSQLUrl:      testPostgresURL,
		RedisUrl:           testRedisURL,
		MaxConcurrentQueue: 10,
		WhitelistTTL:       15 * time.Minute,
		HeartbeatTTL:       5 * time.Minute,
		QueueTTL:           5 * time.Minute,
		PromoteInterval:    50 * time.Millisecond,
	}
	for _, o := range overrides {
		o(cfg)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repos, err := repository.NewRepositories(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("new repositories: %v", err)
	}
	t.Cleanup(repos.Close)

	return repos
}

func resetState(t *testing.T, repos *repository.Repositories) {
	t.Helper()
	ctx := context.Background()

	if _, err := repos.DbPool.Exec(ctx, `TRUNCATE reservations, events, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
	if err := repos.Rdb.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush redis: %v", err)
	}
	repos.ReservationRepository.ClearBlockedEvents()
}

func createTestUser(t *testing.T, repos *repository.Repositories) string {
	t.Helper()
	ctx := context.Background()

	var id string
	err := repos.DbPool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash)
		VALUES ($1, $2, 'test-hash')
		RETURNING id
	`, "Test User", fmt.Sprintf("user-%s@example.com", uuid.NewString())).Scan(&id)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return id
}

func createTestEvent(t *testing.T, repos *repository.Repositories, capacity uint32, maxReservePerUser uint32) *events.EventItem {
	t.Helper()
	ctx := context.Background()

	event, err := repos.EventRepository.CreateEvent(ctx, events.NewEvent{
		Name:              "Test Event",
		Capacity:          capacity,
		MaxReservePerUser: maxReservePerUser,
	})
	if err != nil {
		t.Fatalf("create test event: %v", err)
	}
	return event
}

func newTestCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func waitFor(t *testing.T, timeout, interval time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %s", timeout)
		}
		time.Sleep(interval)
	}
}
