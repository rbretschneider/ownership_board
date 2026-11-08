ALTER TABLE build_hosts ADD COLUMN running_job_ids TEXT NOT NULL DEFAULT '[]' CHECK (CASE WHEN json_valid(running_job_ids) THEN json_type(running_job_ids)='array' AND json_array_length(running_job_ids)<=128 ELSE 0 END);
ALTER TABLE build_hosts ADD COLUMN free_disk_bytes INTEGER CHECK (free_disk_bytes IS NULL OR (typeof(free_disk_bytes)='integer' AND free_disk_bytes>=0));
CREATE TABLE build_host_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 host_id INTEGER NOT NULL REFERENCES build_hosts(id),
 event_type TEXT NOT NULL CHECK(event_type='toolchains_changed'),
 data TEXT NOT NULL CHECK(CASE WHEN json_valid(data) THEN json_type(data)='object' ELSE 0 END),
 created_at TEXT NOT NULL
);
CREATE INDEX build_host_events_host_id ON build_host_events(host_id,id);
