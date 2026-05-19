CREATE TABLE favorites (
    user_id    UUID NOT NULL REFERENCES users(id)       ON DELETE CASCADE,
    blob_id    UUID          REFERENCES stored_blobs(id) ON DELETE CASCADE,
    folder_id  UUID          REFERENCES folders(id)      ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT favorites_one_resource CHECK (
        (blob_id IS NOT NULL)::int + (folder_id IS NOT NULL)::int = 1
    )
);

CREATE UNIQUE INDEX favorites_blob_uidx   ON favorites(user_id, blob_id)   WHERE blob_id   IS NOT NULL;
CREATE UNIQUE INDEX favorites_folder_uidx ON favorites(user_id, folder_id) WHERE folder_id IS NOT NULL;
CREATE INDEX        favorites_user_idx    ON favorites(user_id, created_at DESC);
