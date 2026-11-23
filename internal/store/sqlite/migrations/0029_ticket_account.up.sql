-- A ticket may be pinned to one model account: '' (any), 'codex' or 'claude'.
-- Workers only claim tickets open to their own account, so work one model
-- cannot do (such as image generation, Codex only) waits for the right one.
ALTER TABLE tickets ADD COLUMN account TEXT NOT NULL DEFAULT '' CHECK (account IN ('', 'codex', 'claude'));
