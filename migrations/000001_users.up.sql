CREATE TABLE users (
    id                    UUID        PRIMARY KEY,
    email                 TEXT        NOT NULL UNIQUE,
    srp_salt              TEXT        NOT NULL,
    srp_verifier          TEXT        NOT NULL,
    bcrypt_salt           TEXT        NOT NULL,
    crypto_salt           BYTEA       NOT NULL,
    public_key            BYTEA,
    encrypted_private_key BYTEA,
    kek_encrypted_master  BYTEA,
    kek_encrypted_recovery BYTEA,
    recovery_salt         BYTEA,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_users_email_length CHECK (char_length(email) BETWEEN 3 AND 320)
);
