DELETE FROM users WHERE kind = 'key';

ALTER TABLE users DROP CONSTRAINT users_key_fields_check;

ALTER TABLE users DROP CONSTRAINT users_kind_check;
ALTER TABLE users ADD CONSTRAINT users_kind_check CHECK (kind IN ('passkey'));
