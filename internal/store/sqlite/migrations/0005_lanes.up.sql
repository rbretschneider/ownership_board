-- Four lanes with stages (docs/lanes.md). The stage is the source of truth;
-- the lane is derived from it. Open questions stay open: they are a flag on the
-- ticket now, not a lane. There is no catch-all: a lane this migration does not
-- know makes the lane CASE yield NULL, which tickets.lane (NOT NULL) refuses, so the
-- migration aborts instead of turning unknown tickets into claimable work. The
-- failed run leaves the schema version dirty: fix the data, then run
-- `migrate force 4` before starting the board again.
ALTER TABLE tickets ADD COLUMN stage TEXT;
ALTER TABLE tickets ADD COLUMN design TEXT NOT NULL DEFAULT '';
ALTER TABLE tickets ADD COLUMN blocked_reason TEXT NOT NULL DEFAULT '';
-- Approvals bind to a revision of the text the owner saw.
ALTER TABLE tickets ADD COLUMN spec_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN design_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN approved_spec_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tickets ADD COLUMN approved_design_revision INTEGER NOT NULL DEFAULT 0;

-- Old Ready tickets were never approved, so they wait for the owner.
UPDATE tickets SET stage = CASE lane
    WHEN 'new'         THEN 'refine'
    WHEN 'answers'     THEN 'refine'
    WHEN 'ready'       THEN 'spec_review'
    WHEN 'in_progress' THEN 'build'
    WHEN 'review'      THEN 'pr'
    WHEN 'done'        THEN 'done'
END;

-- Keep the owner's order inside Todo: old Ready first, then New, then old
-- Pending Answers, each group in its existing order.
UPDATE tickets SET rank = rank + CASE lane
    WHEN 'ready'   THEN -1e12
    WHEN 'answers' THEN 1e12
    ELSE 0
END;

UPDATE tickets SET lane = CASE lane
    WHEN 'new'         THEN 'todo'
    WHEN 'answers'     THEN 'todo'
    WHEN 'ready'       THEN 'todo'
    WHEN 'in_progress' THEN 'in_progress'
    WHEN 'review'      THEN 'review'
    WHEN 'done'        THEN 'merged'
END;

UPDATE tickets SET spec_revision = 1 WHERE spec <> '';

-- Every hand-off is a fresh start: no lease survives the change of rules.
UPDATE tickets SET lease_holder = '', lease_expires_at = '';

-- Refuse a ticket the mapping above left without a stage.
CREATE TRIGGER tickets_stage_required_insert BEFORE INSERT ON tickets
WHEN NEW.stage IS NULL BEGIN SELECT RAISE(ABORT, 'ticket stage is required'); END;
CREATE TRIGGER tickets_stage_required_update BEFORE UPDATE ON tickets
WHEN NEW.stage IS NULL BEGIN SELECT RAISE(ABORT, 'ticket stage is required'); END;

-- Checkpoints, set per repo by people. Existing and new repos start with spec
-- approval on and design approval off.
ALTER TABLE repos ADD COLUMN require_spec_approval INTEGER NOT NULL DEFAULT 1;
ALTER TABLE repos ADD COLUMN require_design_approval INTEGER NOT NULL DEFAULT 0;
