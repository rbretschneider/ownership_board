-- Board-wide settings people change in Settings, one row per key. Each value
-- is validated by the server; a missing or unreadable row falls back to the
-- setting's default.
CREATE TABLE board_settings (
    key         TEXT PRIMARY KEY,
    value       TEXT NOT NULL,
    updated_by  TEXT NOT NULL DEFAULT '',
    updated_at  TEXT NOT NULL
);

-- How many autonomous tickets may be in design or build at once.
INSERT INTO board_settings (key, value, updated_by, updated_at)
VALUES ('autonomous_in_flight_cap', '1', '', '2026-10-30T00:00:00.000Z');

-- depends_on is a relation kind like the others (relations.kind has no CHECK);
-- this index serves the claim filter and the waiting_on read.
CREATE INDEX relations_depends_on ON relations (from_id, to_id) WHERE kind = 'depends_on';
