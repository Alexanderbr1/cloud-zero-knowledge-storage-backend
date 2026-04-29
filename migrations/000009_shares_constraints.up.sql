-- Prevent duplicate active shares: one recipient can have at most one active share per blob.
-- Revoked shares (revoked_at IS NOT NULL) are excluded so the same user can be re-invited
-- after revocation.
CREATE UNIQUE INDEX idx_file_shares_active_unique
    ON file_shares (blob_id, recipient_id)
    WHERE revoked_at IS NULL;
