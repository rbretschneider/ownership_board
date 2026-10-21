-- One stored credential per provider. Token columns hold AES-GCM ciphertext
-- (internal/secretbox); generation guards every token rotation with a
-- compare-and-swap.
CREATE TABLE provider_connections (
    provider           TEXT    PRIMARY KEY,
    base_url           TEXT    NOT NULL DEFAULT '',
    account            TEXT    NOT NULL DEFAULT '',
    account_id         TEXT    NOT NULL DEFAULT '',
    email              TEXT    NOT NULL DEFAULT '',
    access_token_enc   TEXT    NOT NULL DEFAULT '',
    refresh_token_enc  TEXT    NOT NULL DEFAULT '',
    access_expires_at  TEXT,
    refresh_expires_at TEXT,
    status             TEXT    NOT NULL,
    generation         INTEGER NOT NULL,
    connected_by       INTEGER NOT NULL REFERENCES users (id),
    connected_at       TEXT    NOT NULL,
    updated_at         TEXT    NOT NULL
);
