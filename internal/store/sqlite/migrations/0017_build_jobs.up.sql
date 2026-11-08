CREATE TABLE build_jobs (
 id TEXT PRIMARY KEY CHECK(length(id)=64 AND id NOT GLOB '*[^a-f0-9]*'),
 ticket_id INTEGER NOT NULL REFERENCES tickets(id),
 repo TEXT NOT NULL,
 sha TEXT NOT NULL,
 preset TEXT NOT NULL CHECK(preset IN ('dotnet','cmake','script')),
 target TEXT NOT NULL CHECK(json_valid(target) AND json_type(target)='object'),
 target_os TEXT NOT NULL CHECK(target_os IN ('linux','windows','macos')),
 status TEXT NOT NULL CHECK(status IN ('queued','running','passed','failed','superseded','cancelled')),
 host_id INTEGER REFERENCES build_hosts(id),
 assignment_released_at TEXT,
 attempt INTEGER NOT NULL CHECK(typeof(attempt)='integer' AND attempt BETWEEN 1 AND 3),
 created_at TEXT NOT NULL,
 started_at TEXT,
 finished_at TEXT,
 phase TEXT NOT NULL DEFAULT '' CHECK(length(CAST(phase AS BLOB))<=64),
 detail TEXT NOT NULL DEFAULT '' CHECK(length(CAST(detail AS BLOB))<=4096),
 result TEXT CHECK(result IS NULL OR (json_valid(result) AND json_type(result)='object' AND length(CAST(result AS BLOB))<=262144)),
 bundle_bytes INTEGER NOT NULL CHECK(bundle_bytes BETWEEN 1 AND 104857600),
 bundle_sha256 TEXT NOT NULL CHECK(length(bundle_sha256)=64 AND bundle_sha256 NOT GLOB '*[^a-f0-9]*')
);
CREATE INDEX build_jobs_selection ON build_jobs(status,target_os,created_at,id);
CREATE INDEX build_jobs_supersession ON build_jobs(ticket_id,target_os,status);
CREATE INDEX build_jobs_assignments ON build_jobs(host_id,assignment_released_at);
CREATE TABLE build_job_uploads (
 token TEXT PRIMARY KEY,
 job_id TEXT NOT NULL CHECK(length(job_id)=64 AND job_id NOT GLOB '*[^a-f0-9]*'),
 metadata TEXT NOT NULL CHECK(json_valid(metadata)),
 reserved_bytes INTEGER NOT NULL CHECK(reserved_bytes BETWEEN 1 AND 104857600),
 created_at TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('active','abandoned'))
);
CREATE UNIQUE INDEX build_job_active_upload ON build_job_uploads(job_id) WHERE state='active';
