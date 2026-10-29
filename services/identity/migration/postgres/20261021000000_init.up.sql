CREATE TABLE users
(
    id             UUID PRIMARY KEY,
    party_id       TEXT        NOT NULL CONSTRAINT users_party_id_key UNIQUE,
    hint           TEXT        NOT NULL CONSTRAINT users_hint_key UNIQUE,
    display_name   TEXT        NOT NULL,
    kind           TEXT        NOT NULL CONSTRAINT users_kind_check CHECK (kind IN ('passkey', 'house')),
    public_key     BYTEA,
    credential_id  TEXT CONSTRAINT users_credential_id_key UNIQUE,
    encrypted_blob TEXT,
    api_key_hash   TEXT CONSTRAINT users_api_key_hash_key UNIQUE,
    maker_mode     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT users_passkey_fields_check
        CHECK (kind <> 'passkey' OR (public_key IS NOT NULL AND credential_id IS NOT NULL AND encrypted_blob IS NOT NULL)),
    CONSTRAINT users_house_fields_check
        CHECK (kind <> 'house' OR api_key_hash IS NOT NULL)
);

CREATE TABLE sessions
(
    token_hash TEXT PRIMARY KEY,
    user_id    UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
