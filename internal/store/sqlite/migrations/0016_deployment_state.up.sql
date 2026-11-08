ALTER TABLE repos ADD COLUMN deploy_mode TEXT NOT NULL DEFAULT 'off' CHECK (deploy_mode IN ('off', 'agent'));
UPDATE tickets SET lane = 'deployed', stage = 'deployed' WHERE lane = 'merged' AND stage = 'done';
