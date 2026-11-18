DELETE FROM build_jobs WHERE kind='flake_check';
ALTER TABLE build_jobs DROP COLUMN bundle_of;
ALTER TABLE build_jobs DROP COLUMN only_tests;
ALTER TABLE build_jobs DROP COLUMN kind;
DROP TABLE test_quarantine;
DROP TABLE test_failures;
