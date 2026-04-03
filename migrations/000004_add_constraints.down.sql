DROP INDEX IF EXISTS idx_refresh_sessions_expires_at;
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_email_length;
ALTER TABLE stored_blobs DROP CONSTRAINT IF EXISTS chk_stored_blobs_upload_method;
