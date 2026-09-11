-- Seed events for local development / staging demos.
--
-- Fixed UUIDs + ON CONFLICT make this file safe to re-run any number of times.
INSERT INTO events (id, name, capacity, max_reserve_per_user, start_sales_at, created_at, updated_at)
VALUES
    ('aaaaaaaa-1111-4aaa-8aaa-aaaaaaaaaaaa', 'Coldplay: Music of the Spheres Tour', 200, 4, now() - interval '1 day', now(), now()),
    ('bbbbbbbb-2222-4bbb-8bbb-bbbbbbbbbbbb', 'Taylor Swift: The Eras Tour',         500, 2, now() + interval '7 days', now(), now()),
    ('cccccccc-3333-4ccc-8ccc-cccccccccccc', 'Jakarta Jazz Festival 2026',          150, 6, now() - interval '1 day', now(), now()),
    ('dddddddd-4444-4ddd-8ddd-dddddddddddd', 'Local Indie Night: The Basement',      50, 8, now() - interval '1 day', now(), now()),
    ('eeeeeeee-5555-4eee-8eee-eeeeeeeeeeee', 'Symphony Under the Stars',            300, 4, now() + interval '30 days', now(), now())
ON CONFLICT (id) DO NOTHING;
