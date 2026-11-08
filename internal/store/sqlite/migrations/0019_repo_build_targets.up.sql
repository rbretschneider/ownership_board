ALTER TABLE repos ADD COLUMN build_targets TEXT NOT NULL DEFAULT '[]'
 CHECK (json_valid(build_targets) AND json_type(build_targets) = 'array' AND json_array_length(build_targets) <= 32);
