-- One verdict per commit and build target: the board's single record of
-- whether a commit passed (engine_lessons.md, "A pass recorded in one place
-- and checked in another").
CREATE TABLE verdicts (
 repo TEXT NOT NULL CHECK(length(repo) BETWEEN 1 AND 512),
 target TEXT NOT NULL CHECK(length(target) BETWEEN 1 AND 1200),
 sha TEXT NOT NULL CHECK(length(sha) IN (40,64) AND sha NOT GLOB '*[^a-f0-9]*'),
 status TEXT NOT NULL CHECK(status IN ('passed','failed','clean_merge')),
 merge_of TEXT NOT NULL DEFAULT '' CHECK(merge_of='' OR (length(merge_of) IN (40,64) AND merge_of NOT GLOB '*[^a-f0-9]*')),
 job_id TEXT NOT NULL CHECK(length(job_id)=64 AND job_id NOT GLOB '*[^a-f0-9]*'),
 ticket_id INTEGER NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
 source TEXT NOT NULL CHECK(source IN ('build_job','clean_merge')),
 reported_by TEXT NOT NULL DEFAULT '' CHECK(length(reported_by)<=200),
 recorded_at TEXT NOT NULL,
 PRIMARY KEY (repo,target,sha),
 CHECK((status='clean_merge') = (merge_of<>'')),
 CHECK((status='clean_merge') = (source='clean_merge'))
);
CREATE INDEX verdicts_ticket ON verdicts(ticket_id,target,recorded_at);
CREATE INDEX verdicts_commit ON verdicts(repo,sha);

-- Carry over the verdicts the build jobs already hold. A failure the host blamed
-- on itself is no verdict. Of several jobs for one commit and target (different
-- presets), the one that finished last wins.
INSERT OR REPLACE INTO verdicts (repo,target,sha,status,merge_of,job_id,ticket_id,source,reported_by,recorded_at)
SELECT repo,
 target_os || COALESCE(':' || (SELECT group_concat(value, ',') FROM (SELECT value FROM json_each(build_jobs.target,'$.required_capabilities') ORDER BY value)), ''),
 sha, status, '', id, ticket_id, 'build_job',
 CASE WHEN host_id IS NULL THEN '' ELSE 'build-host/' || host_id END,
 COALESCE(finished_at, created_at)
FROM build_jobs
WHERE status='passed'
 OR (status='failed' AND COALESCE(json_extract(result,'$.failure_kind'),'') NOT IN ('infrastructure','cancelled'))
ORDER BY COALESCE(finished_at, created_at), id;
