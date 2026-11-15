CREATE TABLE manager_sessions (
    id         INTEGER PRIMARY KEY,
    at         TEXT    NOT NULL,
    model      TEXT    NOT NULL DEFAULT '',
    cost_usd   REAL    NOT NULL DEFAULT 0 CHECK (cost_usd >= 0),
    summary    TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX manager_sessions_at ON manager_sessions (at);
