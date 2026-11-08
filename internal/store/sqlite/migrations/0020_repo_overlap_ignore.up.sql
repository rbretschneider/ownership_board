ALTER TABLE repos ADD COLUMN overlap_ignore TEXT NOT NULL DEFAULT '[]'
CHECK (json_valid(overlap_ignore) AND json_type(overlap_ignore) = 'array' AND json_array_length(overlap_ignore) <= 100);
