-- Migrate to SRP-6a + bcrypt authentication.
-- Existing users cannot be migrated (no verifier was stored),
-- so we truncate and add the new columns.
TRUNCATE TABLE users CASCADE;

ALTER TABLE users
    DROP COLUMN password_hash,
    ADD COLUMN srp_salt     text NOT NULL,
    ADD COLUMN srp_verifier text NOT NULL,
    ADD COLUMN bcrypt_salt  text NOT NULL;
