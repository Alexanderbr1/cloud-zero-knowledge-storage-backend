CREATE TABLE folders (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id  UUID        REFERENCES folders(id) ON DELETE RESTRICT,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chk_folders_name CHECK (char_length(name) BETWEEN 1 AND 255)
);

CREATE UNIQUE INDEX idx_folders_unique_root
    ON folders (user_id, name)
    WHERE parent_id IS NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX idx_folders_unique_parent
    ON folders (user_id, parent_id, name)
    WHERE parent_id IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX idx_folders_user_parent
    ON folders (user_id, parent_id)
    WHERE deleted_at IS NULL;

-- FK index — required for recursive tree-walk CTEs and ON DELETE RESTRICT checks.
CREATE INDEX idx_folders_parent_id
    ON folders (parent_id)
    WHERE parent_id IS NOT NULL;

CREATE INDEX idx_folders_trash
    ON folders (user_id, deleted_at DESC)
    WHERE deleted_at IS NOT NULL;

-- Purge job: WHERE deleted_at < $1 with no user_id filter.
CREATE INDEX idx_folders_purge
    ON folders (deleted_at)
    WHERE deleted_at IS NOT NULL;
