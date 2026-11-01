ALTER TABLE users DROP CONSTRAINT users_kind_check;
ALTER TABLE users ADD CONSTRAINT users_kind_check CHECK (kind IN ('passkey', 'house'));

ALTER TABLE users ADD COLUMN api_key_hash TEXT CONSTRAINT users_api_key_hash_key UNIQUE;
ALTER TABLE users ADD CONSTRAINT users_house_fields_check CHECK (kind <> 'house' OR api_key_hash IS NOT NULL);
