-- Going down turns every autonomous ticket back into one with the repo's
-- checkpoints, and every delegate token into a plain agent token.
ALTER TABLE api_tokens DROP COLUMN is_delegate;
ALTER TABLE tickets DROP COLUMN is_autonomous;
ALTER TABLE repos DROP COLUMN allow_autonomous;
