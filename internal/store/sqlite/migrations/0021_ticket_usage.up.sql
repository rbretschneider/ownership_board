CREATE TABLE ticket_usage (
    id                 INTEGER PRIMARY KEY,
    ticket_id          INTEGER NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    call_key           TEXT    NOT NULL UNIQUE,
    provider           TEXT    NOT NULL,
    model              TEXT    NOT NULL DEFAULT '',
    input_tokens       INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    cache_write_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cache_write_tokens >= 0),
    cache_read_tokens  INTEGER NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
    output_tokens      INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cost_usd           REAL    NOT NULL DEFAULT 0 CHECK (cost_usd >= 0),
    reported_by        TEXT    NOT NULL,
    reported_at        TEXT    NOT NULL
);
CREATE INDEX ticket_usage_ticket ON ticket_usage (ticket_id);
