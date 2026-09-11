DROP INDEX IF EXISTS reservations_event_user_idempotency_key_unique_idx;

CREATE UNIQUE INDEX IF NOT EXISTS reservations_idempotency_key_unique_idx
    ON reservations (idempotency_key)
    WHERE idempotency_key IS NOT NULL;
