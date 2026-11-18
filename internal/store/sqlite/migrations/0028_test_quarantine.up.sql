-- Flaky-test quarantine (docs/amendments/flaky-quarantine.md).

-- Each test that failed in a run, per ticket commit the run verified. A batch
-- run records one row per member it verified.
CREATE TABLE test_failures (
 run_id TEXT NOT NULL CHECK(length(run_id)=64 AND run_id NOT GLOB '*[^a-f0-9]*'),
 repo TEXT NOT NULL,
 ticket_id INTEGER NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
 sha TEXT NOT NULL CHECK(length(sha) IN (40,64) AND sha NOT GLOB '*[^a-f0-9]*'),
 test TEXT NOT NULL CHECK(length(CAST(test AS BLOB)) BETWEEN 1 AND 300),
 recorded_at TEXT NOT NULL,
 PRIMARY KEY(run_id,ticket_id,test)
);
CREATE INDEX test_failures_test ON test_failures(repo,test,recorded_at);
CREATE INDEX test_failures_commit ON test_failures(repo,sha);

-- A repo's quarantined tests. A probation test's fix ticket merged: it runs
-- again, and leaves after enough passing runs.
CREATE TABLE test_quarantine (
 repo_id INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
 test TEXT NOT NULL CHECK(length(CAST(test AS BLOB)) BETWEEN 1 AND 300),
 status TEXT NOT NULL CHECK(status IN ('active','probation','released')),
 source TEXT NOT NULL CHECK(source IN ('detected','seeded')),
 added_by TEXT NOT NULL DEFAULT '' CHECK(length(added_by)<=200),
 reason TEXT NOT NULL DEFAULT '' CHECK(length(CAST(reason AS BLOB))<=2000),
 evidence TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(evidence) AND length(evidence)<=8192),
 fix_ticket_id INTEGER REFERENCES tickets(id) ON DELETE SET NULL,
 passes INTEGER NOT NULL DEFAULT 0 CHECK(passes>=0),
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL,
 released_at TEXT,
 PRIMARY KEY(repo_id,test)
);

-- A flake check re-runs only some tests (only_tests) of a commit, reusing that
-- commit job's bundle (bundle_of). It never gives a verdict.
ALTER TABLE build_jobs ADD COLUMN kind TEXT NOT NULL DEFAULT 'commit' CHECK(kind IN ('commit','flake_check'));
ALTER TABLE build_jobs ADD COLUMN only_tests TEXT CHECK(only_tests IS NULL OR (json_valid(only_tests) AND json_type(only_tests)='array'));
ALTER TABLE build_jobs ADD COLUMN bundle_of TEXT CHECK(bundle_of IS NULL OR (length(bundle_of)=64 AND bundle_of NOT GLOB '*[^a-f0-9]*'));
