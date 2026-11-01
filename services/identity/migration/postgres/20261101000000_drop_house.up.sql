DELETE FROM users WHERE kind = 'house';

ALTER TABLE users DROP CONSTRAINT users_house_fields_check;
ALTER TABLE users DROP COLUMN api_key_hash;

ALTER TABLE users DROP CONSTRAINT users_kind_check;
ALTER TABLE users ADD CONSTRAINT users_kind_check CHECK (kind IN ('passkey'));
