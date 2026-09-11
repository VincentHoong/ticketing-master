package reservations

import (
	"context"
	"errors"
	"fmt"
	"ticketing-master/repository/events"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInsufficientCapacity = errors.New("insufficient capacity")
var ErrExceedMaxReserveQuantity = errors.New("exceed maximum reserve quantity")
var ErrInvalidReservationUpdateStatus = errors.New("invalid reservation update status")

type PostgresReservationRepository struct {
	DbPool *pgxpool.Pool
}

func newPostgresReservationRepository(dbpool *pgxpool.Pool) *PostgresReservationRepository {
	r := &PostgresReservationRepository{DbPool: dbpool}

	return r
}

func (r *PostgresReservationRepository) GetReservation(ctx context.Context, id string) (*ReservationItem, error) {
	pgRows, err := r.DbPool.Query(ctx, `
		SELECT 
			id, 
			user_id,
			event_id,
			quantity,
			status,
			idempotency_key,
			reserved_at, 
			expires_at,
			confirmed_at,
			released_at,
			created_at,
			updated_at
		FROM reservations WHERE id = $1 LIMIT 1
	`, id)
	if err != nil {
		return nil, err
	}

	reservationItem, err := pgx.CollectExactlyOneRow(pgRows, pgx.RowToStructByName[ReservationItem])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReservationNotFound
		}
		return nil, err
	}

	return &reservationItem, nil
}

func (r *PostgresReservationRepository) GetTotalReserved(ctx context.Context, eventId string) (int64, error) {
	var total int64
	err := r.DbPool.QueryRow(ctx, `
		SELECT COALESCE(SUM(quantity), 0)
		FROM reservations
		WHERE event_id = $1
		AND (
			(status = $2 AND expires_at > now())
			OR status = $3
		)
	`, eventId, StatusHeld, StatusConfirmed).Scan(&total)

	return total, err
}

func (r *PostgresReservationRepository) ReserveEvent(ctx context.Context, eventId string, userId string, quantity uint32, idempotencyKey *string) (*ReservationItem, bool, error) {
	tx, err := r.DbPool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	var dbCapacity uint32
	var dbMaxReservePerUser uint32
	err = tx.QueryRow(ctx, `
		SELECT capacity, max_reserve_per_user
		FROM events
		WHERE id = $1
		FOR UPDATE
	`, eventId).Scan(&dbCapacity, &dbMaxReservePerUser)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, events.ErrEventNotFound
		}
		return nil, false, err
	}
	if quantity > dbMaxReservePerUser {
		return nil, false, ErrExceedMaxReserveQuantity
	}

	var dbActiveReserved uint32
	var dbActivePerUserReserved uint32
	err = tx.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(quantity), 0) AS total_reserved,
			COALESCE(SUM(quantity) FILTER (WHERE user_id = $4), 0) AS user_reserved
		FROM reservations
		WHERE event_id = $1
		AND (
			(status = $2 AND expires_at > now())
			OR status = $3
		)
	`, eventId, StatusHeld, StatusConfirmed, userId).Scan(&dbActiveReserved, &dbActivePerUserReserved)
	if err != nil {
		return nil, false, err
	}
	isCapped := dbActiveReserved >= dbCapacity
	if dbActivePerUserReserved+quantity > dbMaxReservePerUser {
		return nil, isCapped, ErrExceedMaxReserveQuantity
	}
	if dbActiveReserved+quantity > dbCapacity {
		return nil, isCapped, ErrInsufficientCapacity
	}

	var id string
	var status ReservationStatus
	reservedAt := time.Now()
	expiresAt := reservedAt.Add(reservationTTL)
	err = tx.QueryRow(ctx, `
		INSERT INTO reservations (
			event_id,
			user_id,
			quantity,
			idempotency_key,
			reserved_at,
			expires_at
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6
		)
		RETURNING id, status;
	`, eventId, userId, quantity, idempotencyKey, reservedAt, expiresAt).Scan(&id, &status)

	isCapped = dbActiveReserved+quantity >= dbCapacity
	if err != nil {
		return nil, isCapped, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, isCapped, err
	}

	return &ReservationItem{
		Id:             id,
		EventId:        eventId,
		UserId:         userId,
		Quantity:       quantity,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		ReservedAt:     &reservedAt,
		ExpiresAt:      &expiresAt,
		CreatedAt:      &reservedAt,
		UpdatedAt:      &reservedAt,
	}, isCapped, nil
}

func (r *PostgresReservationRepository) UpdateReservation(ctx context.Context, userId string, reservationId string, status ReservationStatus) error {
	var column string
	switch status {
	case StatusConfirmed:
		column = "confirmed_at"
	case StatusReleased:
		column = "released_at"
	default:
		return ErrInvalidReservationUpdateStatus
	}

	tx, err := r.DbPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE reservations SET
		status = $1,
		%s = $2
		WHERE id = $3
		AND user_id = $4
		AND status = $5
		AND expires_at > now();
	`, column), status, time.Now(), reservationId, userId, StatusHeld)

	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrReservationNotFound
	}

	return tx.Commit(ctx)
}

func (r *PostgresReservationRepository) RefreshEventStatus(ctx context.Context) (int64, error) {
	tags, err := r.DbPool.Exec(ctx, `
		UPDATE reservations SET
		status = $1,
		released_at = now()
		WHERE status = $2
		AND expires_at <= now()
	`, StatusExpired, StatusHeld)

	if err != nil {
		return 0, err
	}

	return tags.RowsAffected(), nil
}

func (r *PostgresReservationRepository) GetReservationByIdempotencyKey(ctx context.Context, eventId string, userId string, idempotencyKey string) (*ReservationItem, error) {
	pgRows, err := r.DbPool.Query(ctx, `
		SELECT * FROM reservations
		WHERE event_id = $1
		AND user_id = $2
		AND idempotency_key = $3
	`, eventId, userId, idempotencyKey)

	if err != nil {
		return nil, err
	}

	reservationItem, err := pgx.CollectExactlyOneRow(pgRows, pgx.RowToStructByName[ReservationItem])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrReservationNotFound
		}
		return nil, err
	}

	return &reservationItem, nil
}
