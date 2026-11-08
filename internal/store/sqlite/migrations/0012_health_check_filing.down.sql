DROP INDEX tickets_health_check_range;
ALTER TABLE tickets DROP COLUMN is_health_check;
ALTER TABLE tickets DROP COLUMN health_check_merge_sequence;
ALTER TABLE repos DROP COLUMN health_checked_through_merge_sequence;
ALTER TABLE repos DROP COLUMN merge_sequence;
