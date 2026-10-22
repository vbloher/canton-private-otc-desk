BEGIN;

DROP INDEX IF EXISTS actions_pending_ref_idx;
ALTER TABLE actions DROP COLUMN IF EXISTS counterparty;

COMMIT;
