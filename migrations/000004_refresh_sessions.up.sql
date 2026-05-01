-- Single-use refresh tokens tied to a device session.
-- Consuming a token (rotation) revokes the old row and issues a new one.
CREATE TABLE refresh_sessions (
    id                 UUID        PRIMARY KEY,
    user_id            UUID        NOT NULL REFERENCES users(id)           ON DELETE CASCADE,
    device_session_id  UUID        NOT NULL REFERENCES device_sessions(id) ON DELETE CASCADE,
    refresh_token_hash BYTEA       NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at         TIMESTAMPTZ
);

-- ConsumeRefreshSession: point lookup by token hash on active sessions only.
CREATE UNIQUE INDEX idx_refresh_sessions_active_hash
    ON refresh_sessions (refresh_token_hash)
    WHERE revoked_at IS NULL;

-- JOIN / ON DELETE CASCADE on device_session_id.
CREATE INDEX idx_refresh_sessions_device
    ON refresh_sessions (device_session_id);

-- Per-user operations (revoke all, count).
CREATE INDEX idx_refresh_sessions_user_id
    ON refresh_sessions (user_id);

-- Cleanup job: DELETE … WHERE expires_at < now() AND revoked_at IS NULL.
CREATE INDEX idx_refresh_sessions_expires_at
    ON refresh_sessions (expires_at)
    WHERE revoked_at IS NULL;
