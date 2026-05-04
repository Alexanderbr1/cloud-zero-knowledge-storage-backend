DROP INDEX IF EXISTS idx_stored_blobs_user_folder;
ALTER TABLE stored_blobs DROP COLUMN IF EXISTS folder_id;

DROP INDEX IF EXISTS idx_folders_user_parent;
DROP INDEX IF EXISTS idx_folders_unique_parent;
DROP INDEX IF EXISTS idx_folders_unique_root;
DROP TABLE IF EXISTS folders;
