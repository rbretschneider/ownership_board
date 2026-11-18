DROP INDEX build_jobs_batch;
ALTER TABLE build_jobs DROP COLUMN batch_solo;
ALTER TABLE build_jobs DROP COLUMN batch_id;
DROP TABLE build_batch_members;
DROP TABLE build_batches;
ALTER TABLE repos DROP COLUMN batch_max;
ALTER TABLE repos DROP COLUMN batch_verify;
