TRUNCATE TABLE users CASCADE;

ALTER TABLE users
    DROP COLUMN srp_salt,
    DROP COLUMN srp_verifier,
    DROP COLUMN bcrypt_salt,
    ADD COLUMN password_hash text NOT NULL;
