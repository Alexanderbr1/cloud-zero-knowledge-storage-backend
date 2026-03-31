DROP INDEX IF EXISTS idx_stored_blobs_user_id;

ALTER TABLE stored_blobs DROP COLUMN IF EXISTS user_id;

DROP TABLE IF EXISTS users;
