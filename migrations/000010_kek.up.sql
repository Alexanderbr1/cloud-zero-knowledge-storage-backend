ALTER TABLE users
    ADD COLUMN kek_encrypted_master   BYTEA,
    ADD COLUMN kek_encrypted_recovery BYTEA,
    ADD COLUMN recovery_salt          BYTEA;

CREATE TABLE password_reset_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA       NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_prt_user_active
    ON password_reset_tokens (user_id)
    WHERE used_at IS NULL;
