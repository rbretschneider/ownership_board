ALTER TABLE repos ADD COLUMN merge_sequence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE repos ADD COLUMN health_checked_through_merge_sequence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN health_check_merge_sequence INTEGER;
ALTER TABLE tickets ADD COLUMN is_health_check INTEGER NOT NULL DEFAULT 0 CHECK (is_health_check IN (0, 1));

-- Historical merge times are best-effort; the ticket ID breaks equal-time ties.
CREATE TEMP TABLE merge_history AS
SELECT t.id, t.repo_id,
       COALESCE((SELECT MIN(h.created_at) FROM timeline h
                 WHERE h.ticket_id = t.id AND h.type = 'stage'
                   AND json_extract(h.data, '$.to') = 'done'), t.lane_changed_at) AS merged_at
FROM tickets t WHERE t.stage = 'done' AND t.repo_id IS NOT NULL;

WITH ordered AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY repo_id ORDER BY merged_at, id) AS sequence
    FROM merge_history
)
UPDATE tickets SET health_check_merge_sequence = (SELECT sequence FROM ordered WHERE ordered.id = tickets.id)
WHERE id IN (SELECT id FROM ordered);

UPDATE repos SET merge_sequence = COALESCE(
    (SELECT MAX(health_check_merge_sequence) FROM tickets WHERE repo_id = repos.id), 0);
UPDATE repos SET health_checked_through_merge_sequence = COALESCE(
    (SELECT health_check_merge_sequence FROM tickets
     WHERE id = repos.health_checked_through_ticket AND repo_id = repos.id),
    (SELECT MAX(t.health_check_merge_sequence) FROM tickets t JOIN merge_history h ON h.id = t.id
     WHERE t.repo_id = repos.id AND h.merged_at <= repos.health_checked_at), 0);
DROP TABLE merge_history;

CREATE INDEX tickets_health_check_range ON tickets (repo_id, stage, is_health_check, health_check_merge_sequence);
