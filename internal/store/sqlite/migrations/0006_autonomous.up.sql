-- Autonomous tickets (docs/lanes.md): the owner agrees a spec in a chat with a
-- delegate agent, which files it Ready with no checkpoints. Only a token a
-- person marked as a delegate may file one.
ALTER TABLE tickets ADD COLUMN is_autonomous INTEGER NOT NULL DEFAULT 0;
ALTER TABLE api_tokens ADD COLUMN is_delegate INTEGER NOT NULL DEFAULT 0;
-- A repo takes autonomous tickets only once a person allows it.
ALTER TABLE repos ADD COLUMN allow_autonomous INTEGER NOT NULL DEFAULT 0;
