-- Going down deletes every depends_on link and the board settings, so tickets
-- no longer wait for each other. Take a backup first to keep them.
DELETE FROM relations WHERE kind = 'depends_on';
DROP INDEX relations_depends_on;
DROP TABLE board_settings;
