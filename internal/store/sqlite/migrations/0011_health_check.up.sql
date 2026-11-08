ALTER TABLE repos ADD COLUMN health_check_every INTEGER NOT NULL DEFAULT 8;
ALTER TABLE repos ADD COLUMN health_checked_through_ticket INTEGER;
ALTER TABLE repos ADD COLUMN health_checked_at TEXT;
