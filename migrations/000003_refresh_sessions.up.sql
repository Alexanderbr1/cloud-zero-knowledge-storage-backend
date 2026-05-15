CREATE TABLE refresh_sessions (
    id                 UUID        PRIMARY KEY,
    user_id            UUID        NOT NULL REFERENCES users(id)           ON DELETE CASCADE,
    device_session_id  UUID        NOT NULL REFERENCES device_sessions(id) ON DELETE CASCADE,
    refresh_token_hash BYTEA       NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at         TIMESTAMPTZ
);

-- Point lookup by token hash on active sessions only.
CREATE UNIQUE INDEX idx_refresh_sessions_active_hash
    ON refresh_sessions (refresh_token_hash)
    WHERE revoked_at IS NULL;

-- Needed for ON DELETE CASCADE from device_sessions.
CREATE INDEX idx_refresh_sessions_device
    ON refresh_sessions (device_session_id);

CREATE INDEX idx_refresh_sessions_user_id
    ON refresh_sessions (user_id);

-- Cleanup job: DELETE WHERE expires_at < now() AND revoked_at IS NULL.
CREATE INDEX idx_refresh_sessions_expires_at
    ON refresh_sessions (expires_at)
    WHERE revoked_at IS NULL;
