-- EC key pair per user (generated client-side, private key encrypted with master key).
-- NULL means the user registered before this feature was added and needs to re-login.
ALTER TABLE users
    ADD COLUMN public_key            BYTEA,
    ADD COLUMN encrypted_private_key BYTEA;

-- File shares: the owner re-wraps the file key for the recipient using ECIES
-- (ephemeral P-256 ECDH + HKDF-SHA256 + AES-KW).
CREATE TABLE file_shares (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    blob_id          UUID        NOT NULL REFERENCES stored_blobs(id) ON DELETE CASCADE,
    owner_id         UUID        NOT NULL REFERENCES users(id)        ON DELETE CASCADE,
    recipient_id     UUID        NOT NULL REFERENCES users(id)        ON DELETE CASCADE,
    -- Sender's ephemeral P-256 public key (SPKI-encoded).
    ephemeral_pub    BYTEA       NOT NULL,
    -- File key wrapped with AES-KW(HKDF(ECDH(ephemeral_priv, recipient_pub))).
    wrapped_file_key BYTEA       NOT NULL,
    expires_at       TIMESTAMPTZ,
    revoked_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_file_shares_owner_id     ON file_shares(owner_id);
CREATE INDEX idx_file_shares_recipient_id ON file_shares(recipient_id);
CREATE INDEX idx_file_shares_blob_id      ON file_shares(blob_id);
