package users

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolationCode = "23505"

type PostgresUserRepository struct {
	DbPool *pgxpool.Pool
}

func newPostgresUserRepository(dbpool *pgxpool.Pool) *PostgresUserRepository {
	r := &PostgresUserRepository{DbPool: dbpool}

	return r
}

func (r *PostgresUserRepository) CreateUser(ctx context.Context, name string, email string, passwordHash string) (*UserItem, error) {
	pgRows, err := r.DbPool.Query(ctx, `
		INSERT INTO users (name, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, name, email, created_at, updated_at
	`, name, email, passwordHash)
	if err != nil {
		return nil, err
	}

	userItem, err := pgx.CollectExactlyOneRow(pgRows, pgx.RowToStructByName[UserItem])
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return nil, ErrUserEmailTaken
		}
		return nil, err
	}

	return &userItem, nil
}

func (r *PostgresUserRepository) CreateUsers(ctx context.Context, users []NewUser) ([]string, error) {
	if len(users) == 0 {
		return nil, nil
	}

	rows := make([][]any, 0, len(users))
	for _, u := range users {
		rows = append(rows, []any{u.Name, u.Email, u.PasswordHash})
	}

	tx, err := r.DbPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE minted_users (
			name TEXT,
			email TEXT,
			password_hash TEXT
		) ON COMMIT DROP
	`); err != nil {
		return nil, err
	}

	if _, err := tx.CopyFrom(ctx,
		pgx.Identifier{"minted_users"},
		[]string{"name", "email", "password_hash"},
		pgx.CopyFromRows(rows),
	); err != nil {
		return nil, err
	}

	pgRows, err := tx.Query(ctx, `
		INSERT INTO users (name, email, password_hash)
		SELECT name, email, password_hash FROM minted_users
		ON CONFLICT DO NOTHING
		RETURNING id
	`)
	if err != nil {
		return nil, err
	}

	ids, err := pgx.CollectRows(pgRows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return ids, nil
}

func (r *PostgresUserRepository) ListUsers(ctx context.Context, limit uint64) ([]UserItem, error) {
	pgRows, err := r.DbPool.Query(ctx, `
		SELECT id, name, email, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(pgRows, pgx.RowToStructByName[UserItem])
}

func (r *PostgresUserRepository) CountUsers(ctx context.Context) (int64, error) {
	var total int64
	err := r.DbPool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&total)
	return total, err
}

func (r *PostgresUserRepository) GetUser(ctx context.Context, id string) (*UserItem, error) {
	pgRows, err := r.DbPool.Query(ctx, `
		SELECT id, name, email, created_at, updated_at FROM users WHERE id = $1 LIMIT 1
	`, id)
	if err != nil {
		return nil, err
	}

	userItem, err := pgx.CollectExactlyOneRow(pgRows, pgx.RowToStructByName[UserItem])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return &userItem, nil
}
