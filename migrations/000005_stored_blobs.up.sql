CREATE TABLE stored_blobs (
    id                 UUID        PRIMARY KEY,
    user_id            UUID        NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    folder_id          UUID        REFERENCES folders(id)          ON DELETE SET NULL,
    file_name          TEXT        NOT NULL,
    content_type       TEXT        NOT NULL,
    object_key         TEXT        NOT NULL UNIQUE,
    encrypted_file_key BYTEA       NOT NULL,
    chunk_size         INTEGER     NOT NULL,
    file_size_plain    BIGINT      NOT NULL,
    upload_id          TEXT,
    file_size          BIGINT      NOT NULL DEFAULT 0,
    uploaded_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at         TIMESTAMPTZ
);

CREATE INDEX idx_stored_blobs_user_created
    ON stored_blobs (user_id, created_at DESC)
    WHERE deleted_at IS NULL AND uploaded_at IS NOT NULL;

CREATE INDEX idx_stored_blobs_user_folder
    ON stored_blobs (user_id, folder_id, created_at DESC)
    WHERE deleted_at IS NULL AND uploaded_at IS NOT NULL;

-- FK index — required for batch operations on folder subtrees.
CREATE INDEX idx_stored_blobs_folder_id
    ON stored_blobs (folder_id)
    WHERE folder_id IS NOT NULL;

CREATE INDEX idx_stored_blobs_trash
    ON stored_blobs (user_id, deleted_at DESC)
    WHERE deleted_at IS NOT NULL;

-- Purge job: WHERE deleted_at < $1 with no user_id filter.
CREATE INDEX idx_stored_blobs_purge
    ON stored_blobs (deleted_at)
    WHERE deleted_at IS NOT NULL;

-- Orphan cleanup job: pending blobs older than N hours.
CREATE INDEX idx_stored_blobs_orphaned
    ON stored_blobs (created_at)
    WHERE uploaded_at IS NULL AND deleted_at IS NULL;
