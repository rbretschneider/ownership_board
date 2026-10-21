-- PRs and commits that implement a ticket.
CREATE TABLE work_links (
    id             INTEGER PRIMARY KEY,
    ticket_id      INTEGER NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    kind           TEXT    NOT NULL,
    url            TEXT    NOT NULL DEFAULT '',
    sha            TEXT    NOT NULL DEFAULT '',
    repo_id        INTEGER REFERENCES repos (id),
    added_by_kind  TEXT    NOT NULL,
    added_by_id    INTEGER NOT NULL,
    added_by_name  TEXT    NOT NULL,
    added_at       TEXT    NOT NULL,
    -- A PR is unique by URL, a commit by sha.
    UNIQUE (ticket_id, kind, url, sha)
);
CREATE INDEX work_links_sha ON work_links (sha);
CREATE INDEX work_links_url ON work_links (url);

-- What was decided on a ticket and by whom. Rows are never edited except to
-- link a successor or to become a content-free tombstone.
CREATE TABLE decisions (
    id                INTEGER PRIMARY KEY,
    ticket_id         INTEGER NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    summary           TEXT    NOT NULL,
    rationale         TEXT    NOT NULL DEFAULT '',
    alternatives      TEXT    NOT NULL DEFAULT '[]',
    source            TEXT    NOT NULL,
    weight            TEXT    NOT NULL,
    decided_by_kind   TEXT    NOT NULL,
    decided_by_id     INTEGER NOT NULL,
    decided_by_name   TEXT    NOT NULL,
    asked_by          TEXT    NOT NULL DEFAULT '',
    question_id       INTEGER REFERENCES questions (id) ON DELETE SET NULL,
    supersedes_id     INTEGER REFERENCES decisions (id) ON DELETE SET NULL,
    superseded_by_id  INTEGER REFERENCES decisions (id) ON DELETE SET NULL,
    is_removed        INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT    NOT NULL
);
CREATE INDEX decisions_ticket ON decisions (ticket_id, id);

-- Typed, directed links between tickets; deleting either end removes them.
CREATE TABLE relations (
    from_id        INTEGER NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    to_id          INTEGER NOT NULL REFERENCES tickets (id) ON DELETE CASCADE,
    kind           TEXT    NOT NULL,
    added_by_kind  TEXT    NOT NULL,
    added_by_id    INTEGER NOT NULL,
    added_by_name  TEXT    NOT NULL,
    added_at       TEXT    NOT NULL,
    PRIMARY KEY (from_id, to_id, kind),
    CHECK (from_id <> to_id)
);
CREATE INDEX relations_to ON relations (to_id);

-- How a person answered: a tap on Accept recommendations is delegation, not a
-- deliberate choice.
ALTER TABLE questions ADD COLUMN answer_bulk INTEGER NOT NULL DEFAULT 0;

-- A person can force a repo to count as public, e.g. before open-sourcing it.
ALTER TABLE repos ADD COLUMN treat_as_public INTEGER NOT NULL DEFAULT 0;
