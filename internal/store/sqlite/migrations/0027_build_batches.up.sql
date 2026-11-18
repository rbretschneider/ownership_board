-- Batched verification (docs/amendments/batched-verification.md): one
-- build-host run verifies several tickets' commits merged together. Off per
-- repo until a person turns it on.
ALTER TABLE repos ADD COLUMN batch_verify INTEGER NOT NULL DEFAULT 0 CHECK(batch_verify IN (0,1));
ALTER TABLE repos ADD COLUMN batch_max INTEGER NOT NULL DEFAULT 4 CHECK(batch_max BETWEEN 2 AND 8);

CREATE TABLE build_batches (
 id TEXT PRIMARY KEY CHECK(length(id)=64 AND id NOT GLOB '*[^a-f0-9]*'),
 repo TEXT NOT NULL,
 preset TEXT NOT NULL CHECK(preset IN ('dotnet','cmake','script')),
 target TEXT NOT NULL CHECK(json_valid(target) AND json_type(target)='object'),
 target_os TEXT NOT NULL CHECK(target_os IN ('linux','windows','macos')),
 status TEXT NOT NULL CHECK(status IN ('queued','running','passed','failed','dissolved','cancelled')),
 parent_id TEXT REFERENCES build_batches(id),
 host_id INTEGER REFERENCES build_hosts(id),
 assignment_released_at TEXT,
 attempt INTEGER NOT NULL CHECK(typeof(attempt)='integer' AND attempt BETWEEN 1 AND 3),
 created_at TEXT NOT NULL,
 started_at TEXT,
 finished_at TEXT,
 phase TEXT NOT NULL DEFAULT '' CHECK(length(CAST(phase AS BLOB))<=64),
 detail TEXT NOT NULL DEFAULT '' CHECK(length(CAST(detail AS BLOB))<=4096),
 result TEXT CHECK(result IS NULL OR (json_valid(result) AND json_type(result)='object' AND length(CAST(result AS BLOB))<=262144))
);
CREATE INDEX build_batches_queue ON build_batches(status,target_os,created_at,id);
CREATE INDEX build_batches_assignments ON build_batches(host_id) WHERE host_id IS NOT NULL AND assignment_released_at IS NULL;

-- One row per ticket job a batch held, in merge order. outcome records what the
-- batch did for it: passed (verdict fanned out), bisected (sent to a smaller
-- batch or alone after a red batch), conflict (did not merge cleanly, verified
-- alone), superseded (a newer commit or a finished ticket replaced it) or
-- returned (the host failed without a verdict too often; verified alone).
CREATE TABLE build_batch_members (
 batch_id TEXT NOT NULL REFERENCES build_batches(id),
 job_id TEXT NOT NULL REFERENCES build_jobs(id) ON DELETE CASCADE,
 position INTEGER NOT NULL CHECK(position BETWEEN 0 AND 7),
 outcome TEXT NOT NULL CHECK(outcome IN ('pending','passed','bisected','conflict','superseded','returned')),
 PRIMARY KEY(batch_id,job_id)
);
CREATE INDEX build_batch_members_job ON build_batch_members(job_id);

-- batch_id: the open batch holding a queued job (not claimable on its own while set).
-- batch_solo: the job is verified alone from now on (conflict, bisected to one, host failures).
ALTER TABLE build_jobs ADD COLUMN batch_id TEXT CHECK(batch_id IS NULL OR (length(batch_id)=64 AND batch_id NOT GLOB '*[^a-f0-9]*'));
ALTER TABLE build_jobs ADD COLUMN batch_solo INTEGER NOT NULL DEFAULT 0 CHECK(batch_solo IN (0,1));
CREATE INDEX build_jobs_batch ON build_jobs(batch_id) WHERE batch_id IS NOT NULL;
