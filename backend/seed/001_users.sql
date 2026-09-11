-- Seed users for local development / staging demos.
--
-- All accounts share the dev-only password "Password123!" (bcrypt hash below).
-- Never run this file against a production database.
--
-- Fixed UUIDs + ON CONFLICT make this file safe to re-run any number of times.
INSERT INTO users (id, name, email, password_hash, created_at, updated_at)
VALUES
    ('11111111-1111-4111-8111-111111111111', 'Ava Thompson',   'ava.thompson@example.com',   '$2a$10$GdGI14Uzy6WzBWBjujbZD.W5VI8K51Ur3lrRpdfknbzsjeTZD/Wjq', now(), now()),
    ('22222222-2222-4222-8222-222222222222', 'Marcus Lee',     'marcus.lee@example.com',     '$2a$10$GdGI14Uzy6WzBWBjujbZD.W5VI8K51Ur3lrRpdfknbzsjeTZD/Wjq', now(), now()),
    ('33333333-3333-4333-8333-333333333333', 'Priya Sharma',   'priya.sharma@example.com',   '$2a$10$GdGI14Uzy6WzBWBjujbZD.W5VI8K51Ur3lrRpdfknbzsjeTZD/Wjq', now(), now()),
    ('44444444-4444-4444-8444-444444444444', 'Diego Fernandez','diego.fernandez@example.com','$2a$10$GdGI14Uzy6WzBWBjujbZD.W5VI8K51Ur3lrRpdfknbzsjeTZD/Wjq', now(), now()),
    ('55555555-5555-4555-8555-555555555555', 'Grace Kim',      'grace.kim@example.com',      '$2a$10$GdGI14Uzy6WzBWBjujbZD.W5VI8K51Ur3lrRpdfknbzsjeTZD/Wjq', now(), now())
ON CONFLICT (id) DO NOTHING;
