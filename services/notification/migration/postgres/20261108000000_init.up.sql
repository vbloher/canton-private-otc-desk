CREATE TABLE profiles
(
    party_id     TEXT PRIMARY KEY,
    email        TEXT        NULL,
    confirmed_at TIMESTAMPTZ NULL,
    news         BOOLEAN     NOT NULL DEFAULT FALSE,
    trades       BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX profiles_email_idx ON profiles (email) WHERE email IS NOT NULL;

CREATE TABLE email_confirmations
(
    token_hash   TEXT PRIMARY KEY,
    party_id     TEXT        NOT NULL REFERENCES profiles (party_id) ON DELETE CASCADE,
    email        TEXT        NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    confirmed_at TIMESTAMPTZ NULL
);

CREATE INDEX email_confirmations_party_idx ON email_confirmations (party_id);
CREATE INDEX email_confirmations_expires_idx ON email_confirmations (expires_at);

CREATE TABLE emails
(
    id              UUID PRIMARY KEY,
    dedupe_key      TEXT        NOT NULL CONSTRAINT emails_dedupe_key_key UNIQUE,
    recipient       TEXT        NOT NULL,
    kind            TEXT        NOT NULL,
    subject         TEXT        NOT NULL,
    html            TEXT        NOT NULL,
    text            TEXT        NOT NULL,
    status          TEXT        NOT NULL CONSTRAINT emails_status_check CHECK (status IN ('pending', 'sent', 'failed')),
    attempts        INT         NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    provider_id     TEXT        NOT NULL DEFAULT '',
    last_error      TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ NULL
);

CREATE INDEX emails_due_idx ON emails (next_attempt_at) WHERE status = 'pending';
CREATE INDEX emails_finished_idx ON emails (finished_at) WHERE status <> 'pending';

CREATE TABLE contact_syncs
(
    email           TEXT PRIMARY KEY,
    attempts        INT         NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX contact_syncs_due_idx ON contact_syncs (next_attempt_at);
