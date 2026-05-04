CREATE TABLE folders (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id  UUID        REFERENCES folders(id) ON DELETE RESTRICT,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_folders_name CHECK (char_length(name) BETWEEN 1 AND 255)
);

-- Unique folder name per user at root level (parent_id IS NULL)
CREATE UNIQUE INDEX idx_folders_unique_root
    ON folders (user_id, name)
    WHERE parent_id IS NULL;

-- Unique folder name per user inside a specific parent
CREATE UNIQUE INDEX idx_folders_unique_parent
    ON folders (user_id, parent_id, name)
    WHERE parent_id IS NOT NULL;

-- ListFolders: WHERE user_id = $1 AND parent_id = $2 (or IS NULL)
CREATE INDEX idx_folders_user_parent ON folders (user_id, parent_id);

-- Folder reference on blobs (NULL = root level)
ALTER TABLE stored_blobs
    ADD COLUMN folder_id UUID REFERENCES folders(id) ON DELETE SET NULL;

-- ListBlobsInFolder: WHERE user_id = $1 AND folder_id = $2 (or IS NULL)
CREATE INDEX idx_stored_blobs_user_folder ON stored_blobs (user_id, folder_id, created_at DESC);
