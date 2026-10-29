-- Every repo takes autonomous tickets; the per-repo opt-in is gone.
ALTER TABLE repos DROP COLUMN allow_autonomous;
