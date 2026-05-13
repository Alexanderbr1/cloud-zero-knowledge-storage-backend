-- Trash (soft delete) support for files and folders.

-- stored_blobs: add soft-delete fields
ALTER TABLE stored_blobs
    ADD COLUMN deleted_at         TIMESTAMPTZ,
    ADD COLUMN original_folder_id UUID REFERENCES folders(id) ON DELETE SET NULL;

-- folders: add soft-delete fields
ALTER TABLE folders
    ADD COLUMN deleted_at          TIMESTAMPTZ,
    ADD COLUMN original_parent_id  UUID REFERENCES folders(id) ON DELETE SET NULL;

-- Recreate unique folder name indexes as partial (exclude deleted rows).
-- Without this a user couldn't create "Photos" after trashing an old "Photos" folder.
DROP INDEX idx_folders_unique_root;
DROP INDEX idx_folders_unique_parent;

CREATE UNIQUE INDEX idx_folders_unique_root
    ON folders (user_id, name)
    WHERE parent_id IS NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX idx_folders_unique_parent
    ON folders (user_id, parent_id, name)
    WHERE parent_id IS NOT NULL AND deleted_at IS NULL;

-- Fast queries for the trash listing page
CREATE INDEX idx_stored_blobs_trash ON stored_blobs (user_id, deleted_at DESC) WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_folders_trash      ON folders      (user_id, deleted_at DESC) WHERE deleted_at IS NOT NULL;
