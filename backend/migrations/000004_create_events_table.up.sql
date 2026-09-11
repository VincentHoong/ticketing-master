CREATE TABLE IF NOT EXISTS events (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                  TEXT NOT NULL,
    capacity              INTEGER NOT NULL,
    max_reserve_per_user  INTEGER NOT NULL,
    -- Second precision is all this needs: sales windows are scheduled/compared
    -- at whole-second granularity, so TIMESTAMPTZ(0) avoids implying sub-second
    -- accuracy that nothing downstream uses or guarantees.
    start_sales_at        TIMESTAMPTZ(0),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at            TIMESTAMPTZ,

    CONSTRAINT events_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT events_capacity_positive CHECK (capacity > 0),
    CONSTRAINT events_max_reserve_per_user_positive CHECK (max_reserve_per_user > 0)
);

-- Most reads filter out soft-deleted events; a partial index keeps that lookup cheap
-- without bloating the index with rows nobody queries for.
CREATE INDEX IF NOT EXISTS events_active_idx ON events (id) WHERE deleted_at IS NULL;

CREATE TRIGGER events_set_updated_at
    BEFORE UPDATE ON events
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();
