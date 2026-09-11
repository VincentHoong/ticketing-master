-- Scope the idempotency key to (event_id, user_id) instead of globally.
--
-- A global unique index let one user's key collide with another's, and once the
-- reserve endpoint replays an existing reservation for a known key, a global
-- index would let a caller read someone else's reservation by guessing it.
-- Scoping the index keeps a key meaningful only within its own event+user.
DROP INDEX IF EXISTS reservations_idempotency_key_unique_idx;

CREATE UNIQUE INDEX IF NOT EXISTS reservations_event_user_idempotency_key_unique_idx
    ON reservations (event_id, user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
