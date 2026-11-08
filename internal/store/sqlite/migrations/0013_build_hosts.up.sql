CREATE TABLE build_hosts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 name TEXT NOT NULL,
 os TEXT NOT NULL CHECK (os IN ('windows','linux','macos')),
 arch TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','revoked')),
 capabilities TEXT NOT NULL DEFAULT '[]' CHECK (CASE WHEN json_valid(capabilities) THEN json_type(capabilities) = 'array' AND json_array_length(capabilities) <= 32 ELSE 0 END),
 max_parallel INTEGER NOT NULL DEFAULT 1 CHECK (typeof(max_parallel) = 'integer' AND max_parallel BETWEEN 1 AND 8),
 enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
 public_key TEXT NOT NULL UNIQUE,
 facts TEXT NOT NULL DEFAULT '{}' CHECK (CASE WHEN json_valid(facts) THEN json_type(facts) = 'object' AND length(CAST(facts AS BLOB)) <= 65536 ELSE 0 END),
 agent_version TEXT NOT NULL DEFAULT '',
 last_seen_at TEXT,
 created_at TEXT NOT NULL,
 approved_at TEXT,
 approved_by INTEGER REFERENCES users(id),
 revoked_at TEXT,
 CHECK ((approved_at IS NULL) = (approved_by IS NULL)),
 CHECK (status != 'approved' OR approved_at IS NOT NULL),
 CHECK (status != 'pending' OR (approved_at IS NULL AND revoked_at IS NULL)),
 CHECK ((status = 'revoked' AND revoked_at IS NOT NULL AND enabled = 0) OR (status != 'revoked' AND revoked_at IS NULL))
);
CREATE INDEX build_hosts_status_id ON build_hosts(status,id);
CREATE TABLE build_host_pairing_codes (
 code_hash TEXT PRIMARY KEY NOT NULL CHECK (length(code_hash) = 64 AND code_hash NOT GLOB '*[^0-9a-f]*'),
 created_by INTEGER NOT NULL REFERENCES users(id),
 created_at TEXT NOT NULL,
 expires_at TEXT NOT NULL,
 used_at TEXT,
 used_by_host INTEGER REFERENCES build_hosts(id),
 CHECK ((used_at IS NULL) = (used_by_host IS NULL))
);
