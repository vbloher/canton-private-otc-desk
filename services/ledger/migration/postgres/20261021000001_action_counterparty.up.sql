BEGIN;

ALTER TABLE actions ADD COLUMN IF NOT EXISTS counterparty TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS actions_pending_ref_idx ON actions (ref) WHERE status = 'pending';

COMMIT;
