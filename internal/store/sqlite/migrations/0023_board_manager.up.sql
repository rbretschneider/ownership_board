ALTER TABLE api_tokens ADD COLUMN is_manager INTEGER NOT NULL DEFAULT 0;
CREATE TABLE manager_usage (
    day        TEXT    PRIMARY KEY CHECK (length(day) = 10),
    sessions   INTEGER NOT NULL DEFAULT 0 CHECK (sessions >= 0),
    cost_usd   REAL    NOT NULL DEFAULT 0 CHECK (cost_usd >= 0),
    updated_at TEXT    NOT NULL
);
