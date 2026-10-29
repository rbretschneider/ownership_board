-- Going down brings the opt-in back, off for every repo.
ALTER TABLE repos ADD COLUMN allow_autonomous INTEGER NOT NULL DEFAULT 0;
