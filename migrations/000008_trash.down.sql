DROP INDEX IF EXISTS idx_folders_trash;
DROP INDEX IF EXISTS idx_stored_blobs_trash;

DROP INDEX IF EXISTS idx_folders_unique_parent;
DROP INDEX IF EXISTS idx_folders_unique_root;

CREATE UNIQUE INDEX idx_folders_unique_root
    ON folders (user_id, name)
    WHERE parent_id IS NULL;

CREATE UNIQUE INDEX idx_folders_unique_parent
    ON folders (user_id, parent_id, name)
    WHERE parent_id IS NOT NULL;

ALTER TABLE folders      DROP COLUMN IF EXISTS original_parent_id, DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE stored_blobs DROP COLUMN IF EXISTS original_folder_id, DROP COLUMN IF EXISTS deleted_at;
