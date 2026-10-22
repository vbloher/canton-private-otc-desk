BEGIN;

-- Sign queue: prepared transactions of external parties waiting for their signature.
CREATE TABLE IF NOT EXISTS actions
(
    id                     UUID PRIMARY KEY,
    party_id               TEXT        NOT NULL,
    purpose                TEXT        NOT NULL,
    description            TEXT        NOT NULL,
    ref                    TEXT        NOT NULL,
    hash                   BYTEA       NOT NULL,
    prepared_transaction   TEXT        NOT NULL,
    hashing_scheme_version TEXT        NOT NULL,
    status                 TEXT        NOT NULL,
    error                  TEXT        NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL,
    expires_at             TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS actions_party_id_status_idx ON actions (party_id, status);

COMMIT;
