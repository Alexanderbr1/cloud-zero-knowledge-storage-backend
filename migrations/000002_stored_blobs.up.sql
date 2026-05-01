-- Files uploaded by users. The content is stored encrypted in MinIO;
-- only the metadata and key material are kept here.
CREATE TABLE stored_blobs (
    id                 UUID        PRIMARY KEY,
    user_id            UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name          TEXT        NOT NULL,
    content_type       TEXT        NOT NULL,
    object_key         TEXT        NOT NULL UNIQUE,
    encrypted_file_key BYTEA       NOT NULL,
    file_iv            BYTEA       NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

);

-- ListBlobs: WHERE user_id = $1 ORDER BY created_at DESC
CREATE INDEX idx_stored_blobs_user_created ON stored_blobs (user_id, created_at DESC);
