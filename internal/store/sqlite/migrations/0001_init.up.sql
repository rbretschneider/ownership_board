CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    name          TEXT    NOT NULL,
    email         TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    -- Empty for an account that only signs in through SSO.
    password_hash TEXT    NOT NULL DEFAULT '',
    is_admin      INTEGER NOT NULL DEFAULT 0,
    -- Bumped to revoke every outstanding token for the user at once.
    token_version INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT    NOT NULL
);

-- SSO identities are linked by (issuer, subject), never by email.
CREATE TABLE identity_links (
    issuer     TEXT    NOT NULL,
    subject    TEXT    NOT NULL,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TEXT    NOT NULL,
    PRIMARY KEY (issuer, subject)
);
CREATE INDEX identity_links_user ON identity_links (user_id);

CREATE TABLE refresh_tokens (
    id            INTEGER PRIMARY KEY,
    user_id       INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash    TEXT    NOT NULL UNIQUE,
    token_version INTEGER NOT NULL,
    expires_at    TEXT    NOT NULL,
    revoked_at    TEXT,
    created_at    TEXT    NOT NULL
);
CREATE INDEX refresh_tokens_user ON refresh_tokens (user_id);

CREATE TABLE api_tokens (
    id           INTEGER PRIMARY KEY,
    name         TEXT    NOT NULL UNIQUE,
    prefix       TEXT    NOT NULL,
    token_hash   TEXT    NOT NULL UNIQUE,
    created_by   INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TEXT    NOT NULL,
    last_used_at TEXT,
    revoked_at   TEXT
);

-- A repo is listed by a provider (or added by hand). The same full name may
-- exist on two providers when one mirrors the other.
CREATE TABLE repos (
    id             INTEGER PRIMARY KEY,
    provider       TEXT    NOT NULL,
    full_name      TEXT    NOT NULL,
    url            TEXT    NOT NULL DEFAULT '',
    description    TEXT    NOT NULL DEFAULT '',
    color          TEXT    NOT NULL DEFAULT '',
    default_branch TEXT    NOT NULL DEFAULT '',
    is_private     INTEGER NOT NULL DEFAULT 0,
    is_archived    INTEGER NOT NULL DEFAULT 0,
    synced_at      TEXT,
    created_at     TEXT    NOT NULL,
    UNIQUE (provider, full_name)
);
CREATE INDEX repos_full_name ON repos (full_name);

CREATE TABLE repo_source_status (
    provider       TEXT PRIMARY KEY,
    last_synced_at TEXT NOT NULL,
    last_error     TEXT NOT NULL DEFAULT ''
);

-- AUTOINCREMENT so a deleted ticket's number is never handed out again.
CREATE TABLE tickets (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    -- NULL until the ticket has a repo.
    repo_id          INTEGER REFERENCES repos (id),
    kind             TEXT    NOT NULL,
    title            TEXT    NOT NULL,
    body             TEXT    NOT NULL DEFAULT '',
    spec             TEXT    NOT NULL DEFAULT '',
    lane             TEXT    NOT NULL,
    rank             REAL    NOT NULL,
    pr_url           TEXT    NOT NULL DEFAULT '',
    branch           TEXT    NOT NULL DEFAULT '',
    lease_holder     TEXT    NOT NULL DEFAULT '',
    lease_expires_at TEXT    NOT NULL DEFAULT '',
    created_by       TEXT    NOT NULL,
    created_at       TEXT    NOT NULL,
    updated_at       TEXT    NOT NULL,
    lane_changed_at  TEXT    NOT NULL
);
CREATE INDEX tickets_lane_rank ON tickets (lane, rank);
CREATE INDEX tickets_repo ON tickets (repo_id);

CREATE TABLE questions (
    id              INTEGER PRIMARY KEY,
    ticket_id       INTEGER NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    round           INTEGER NOT NULL,
    position        INTEGER NOT NULL,
    prompt          TEXT    NOT NULL,
    options         TEXT    NOT NULL DEFAULT '[]',
    recommended     INTEGER,
    asked_by        TEXT    NOT NULL,
    asked_at        TEXT    NOT NULL,
    answer_choice   INTEGER,
    answer_text     TEXT    NOT NULL DEFAULT '',
    answer_deferred INTEGER NOT NULL DEFAULT 0,
    answered_by     TEXT    NOT NULL DEFAULT '',
    answered_at     TEXT
);
CREATE INDEX questions_ticket ON questions (ticket_id, round, position);

CREATE TABLE timeline (
    id         INTEGER PRIMARY KEY,
    ticket_id  INTEGER NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    type       TEXT    NOT NULL,
    actor_kind TEXT    NOT NULL,
    actor_name TEXT    NOT NULL,
    body       TEXT    NOT NULL DEFAULT '',
    data       TEXT    NOT NULL DEFAULT '{}',
    created_at TEXT    NOT NULL
);
CREATE INDEX timeline_ticket ON timeline (ticket_id, id);
