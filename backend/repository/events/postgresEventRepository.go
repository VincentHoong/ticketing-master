package events

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresEventRepository struct {
	DbPool *pgxpool.Pool
}

func newPostgresEventRepository(dbpool *pgxpool.Pool) *PostgresEventRepository {
	e := &PostgresEventRepository{DbPool: dbpool}

	return e
}

func (e *PostgresEventRepository) CreateEvent(ctx context.Context, event NewEvent) (*EventItem, error) {
	pgRows, err := e.DbPool.Query(ctx, `
		INSERT INTO events (name, capacity, max_reserve_per_user, start_sales_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, max_reserve_per_user, capacity, start_sales_at, created_at, updated_at
	`, event.Name, event.Capacity, event.MaxReservePerUser, event.StartSalesAt)
	if err != nil {
		return nil, err
	}

	eventItem, err := pgx.CollectExactlyOneRow(pgRows, pgx.RowToStructByName[EventItem])
	if err != nil {
		return nil, err
	}

	return &eventItem, nil
}

func (e *PostgresEventRepository) CreateEvents(ctx context.Context, events []NewEvent) ([]EventItem, error) {
	if len(events) == 0 {
		return nil, nil
	}

	rows := make([][]any, 0, len(events))
	for _, ev := range events {
		rows = append(rows, []any{ev.Name, ev.Capacity, ev.MaxReservePerUser, ev.StartSalesAt})
	}

	tx, err := e.DbPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE minted_events (
			name TEXT,
			capacity INTEGER,
			max_reserve_per_user INTEGER,
			start_sales_at TIMESTAMPTZ
		) ON COMMIT DROP
	`); err != nil {
		return nil, err
	}

	if _, err := tx.CopyFrom(ctx,
		pgx.Identifier{"minted_events"},
		[]string{"name", "capacity", "max_reserve_per_user", "start_sales_at"},
		pgx.CopyFromRows(rows),
	); err != nil {
		return nil, err
	}

	pgRows, err := tx.Query(ctx, `
		INSERT INTO events (name, capacity, max_reserve_per_user, start_sales_at)
		SELECT name, capacity, max_reserve_per_user, start_sales_at FROM minted_events
		RETURNING id, name, max_reserve_per_user, capacity, start_sales_at, created_at, updated_at
	`)
	if err != nil {
		return nil, err
	}

	created, err := pgx.CollectRows(pgRows, pgx.RowToStructByName[EventItem])
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return created, nil
}

func (e *PostgresEventRepository) ListEvents(ctx context.Context, limit uint64) ([]EventItem, error) {
	pgRows, err := e.DbPool.Query(ctx, `
		SELECT
			id,
			name,
			max_reserve_per_user,
			capacity,
			start_sales_at,
			created_at,
			updated_at
		FROM events
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(pgRows, pgx.RowToStructByName[EventItem])
}

func (e *PostgresEventRepository) GetEvent(ctx context.Context, id string) (*EventItem, error) {
	pgRows, err := e.DbPool.Query(ctx, `
		SELECT 
			id,
			name,
			max_reserve_per_user,
			capacity,
			start_sales_at,
			created_at,
			updated_at
		FROM events WHERE id = $1 LIMIT 1
	`, id)
	if err != nil {
		return nil, err
	}

	eventItem, err := pgx.CollectExactlyOneRow(pgRows, pgx.RowToStructByName[EventItem])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}

	return &eventItem, nil
}
