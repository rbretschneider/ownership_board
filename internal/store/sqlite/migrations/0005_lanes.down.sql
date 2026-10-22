-- WARNING: going down drops designs, blocked reasons, revisions and the
-- per-repo checkpoint settings. Take a backup first.
DROP TRIGGER tickets_stage_required_update;
DROP TRIGGER tickets_stage_required_insert;
UPDATE tickets SET lane = CASE stage
    WHEN 'refine'        THEN 'new'
    WHEN 'spec_review'   THEN 'new'
    WHEN 'ready'         THEN 'ready'
    WHEN 'design'        THEN 'in_progress'
    WHEN 'design_review' THEN 'in_progress'
    WHEN 'build'         THEN 'in_progress'
    WHEN 'pr'            THEN 'review'
    WHEN 'done'          THEN 'done'
END;
ALTER TABLE repos DROP COLUMN require_design_approval;
ALTER TABLE repos DROP COLUMN require_spec_approval;
ALTER TABLE tickets DROP COLUMN approved_design_revision;
ALTER TABLE tickets DROP COLUMN approved_spec_revision;
ALTER TABLE tickets DROP COLUMN design_revision;
ALTER TABLE tickets DROP COLUMN spec_revision;
ALTER TABLE tickets DROP COLUMN blocked_reason;
ALTER TABLE tickets DROP COLUMN design;
ALTER TABLE tickets DROP COLUMN stage;
