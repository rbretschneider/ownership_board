CREATE INDEX build_jobs_active_assignments ON build_jobs(host_id) WHERE host_id IS NOT NULL AND assignment_released_at IS NULL;
CREATE INDEX build_jobs_commit_history ON build_jobs(ticket_id,target,sha);
