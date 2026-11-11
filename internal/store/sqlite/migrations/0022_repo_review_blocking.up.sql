ALTER TABLE repos ADD COLUMN review_blocking TEXT NOT NULL DEFAULT 'p1' CHECK (review_blocking IN ('p0', 'p1', 'p2', 'p3'));
