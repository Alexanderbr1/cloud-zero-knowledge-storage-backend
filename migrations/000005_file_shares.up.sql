-- File shares: the owner re-wraps the file key for the recipient using ECIES
-- (ephemeral P-256 ECDH + HKDF-SHA256 + AES-KW). The recipient decrypts it
-- with their private key to recover the file key.
CREATE TABLE file_shares (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    blob_id          UUID        NOT NULL REFERENCES stored_blobs(id) ON DELETE CASCADE,
    owner_id         UUID        NOT NULL REFERENCES users(id)        ON DELETE CASCADE,
    recipient_id     UUID        NOT NULL REFERENCES users(id)        ON DELETE CASCADE,
    ephemeral_pub    BYTEA       NOT NULL,
    wrapped_file_key BYTEA       NOT NULL,
    expires_at       TIMESTAMPTZ,
    revoked_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Plain FK indexes — must cover all rows including revoked for ON DELETE CASCADE.
CREATE INDEX idx_file_shares_owner_id     ON file_shares (owner_id);
CREATE INDEX idx_file_shares_recipient_id ON file_shares (recipient_id);
CREATE INDEX idx_file_shares_blob_id      ON file_shares (blob_id);

-- ListSharedWithUser: WHERE recipient_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC
CREATE INDEX idx_file_shares_recipient_active
    ON file_shares (recipient_id, created_at DESC)
    WHERE revoked_at IS NULL;

-- One active share per blob+recipient pair; revoked shares are excluded so
-- the same user can be re-invited after revocation.
CREATE UNIQUE INDEX idx_file_shares_active_unique
    ON file_shares (blob_id, recipient_id)
    WHERE revoked_at IS NULL;
