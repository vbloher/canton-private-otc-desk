BEGIN;

-- The other party whose wallet an action's outcome changes: a withdrawal's receiver, an
-- accepted transfer's sender. It is told when the action executes.
ALTER TABLE actions ADD COLUMN IF NOT EXISTS counterparty TEXT NOT NULL DEFAULT '';

-- Cancelling a settlement expires its pending actions by ref.
CREATE INDEX IF NOT EXISTS actions_pending_ref_idx ON actions (ref) WHERE status = 'pending';

COMMIT;
