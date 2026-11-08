UPDATE tickets SET lane = 'merged', stage = 'done' WHERE lane = 'deployed' AND stage = 'deployed';
ALTER TABLE repos DROP COLUMN deploy_mode;
