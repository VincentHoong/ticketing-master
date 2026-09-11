-- A reservation is a temporary hold on N seats for an event that either
-- expires, gets confirmed (sold), or is manually released.
CREATE TABLE IF NOT EXISTS reservations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id         UUID NOT NULL REFERENCES events (id),
    user_id          UUID NOT NULL REFERENCES users (id),
    quantity         INTEGER NOT NULL,
    status           TEXT NOT NULL DEFAULT 'held',
    idempotency_key  TEXT,
    reserved_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at       TIMESTAMPTZ NOT NULL,
    confirmed_at     TIMESTAMPTZ,
    released_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT reservations_quantity_positive CHECK (quantity > 0),
    CONSTRAINT reservations_status_valid CHECK (status IN ('held', 'confirmed', 'released', 'expired'))
);

-- One row per confirm attempt key: lets the confirm endpoint be retried
-- safely without double-selling on a client retry.
CREATE UNIQUE INDEX IF NOT EXISTS reservations_idempotency_key_unique_idx
    ON reservations (idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Oversell checks sum quantity for an event's active (held/confirmed) reservations.
CREATE INDEX IF NOT EXISTS reservations_event_status_idx ON reservations (event_id, status);

CREATE INDEX IF NOT EXISTS reservations_user_idx ON reservations (user_id);

-- Lets a background sweeper find expired holds cheaply without scanning confirmed/released rows.
CREATE INDEX IF NOT EXISTS reservations_expiring_holds_idx
    ON reservations (expires_at)
    WHERE status = 'held';

CREATE TRIGGER reservations_set_updated_at
    BEFORE UPDATE ON reservations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
