ALTER TABLE users DROP CONSTRAINT users_kind_check;
ALTER TABLE users ADD CONSTRAINT users_kind_check CHECK (kind IN ('passkey', 'key'));

ALTER TABLE users ADD CONSTRAINT users_key_fields_check
    CHECK (kind <> 'key' OR (public_key IS NOT NULL AND credential_id IS NULL AND encrypted_blob IS NULL));
