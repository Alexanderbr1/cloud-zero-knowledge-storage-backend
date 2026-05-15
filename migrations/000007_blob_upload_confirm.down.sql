DROP INDEX IF EXISTS idx_stored_blobs_orphaned;
DROP INDEX IF EXISTS idx_stored_blobs_user_created;
DROP INDEX IF EXISTS idx_stored_blobs_user_folder;

ALTER TABLE stored_blobs DROP COLUMN uploaded_at;

CREATE INDEX idx_stored_blobs_user_created
    ON stored_blobs (user_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_stored_blobs_user_folder
    ON stored_blobs (user_id, folder_id, created_at DESC)
    WHERE deleted_at IS NULL;
