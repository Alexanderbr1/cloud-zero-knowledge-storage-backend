ALTER TABLE stored_blobs ADD COLUMN uploaded_at TIMESTAMPTZ;

-- Existing blobs were uploaded before this migration — mark them confirmed.
UPDATE stored_blobs SET uploaded_at = created_at;

-- Rebuild listing indexes to exclude pending (unconfirmed) blobs.
DROP INDEX idx_stored_blobs_user_created;
DROP INDEX idx_stored_blobs_user_folder;

CREATE INDEX idx_stored_blobs_user_created
    ON stored_blobs (user_id, created_at DESC)
    WHERE deleted_at IS NULL AND uploaded_at IS NOT NULL;

CREATE INDEX idx_stored_blobs_user_folder
    ON stored_blobs (user_id, folder_id, created_at DESC)
    WHERE deleted_at IS NULL AND uploaded_at IS NOT NULL;

-- Orphan cleanup job: pending blobs older than N hours.
CREATE INDEX idx_stored_blobs_orphaned
    ON stored_blobs (created_at)
    WHERE uploaded_at IS NULL AND deleted_at IS NULL;
