-- One row per logged-in device. Survives refresh token rotation:
-- subsequent refreshes update last_active_at but do not create a new row.
CREATE TABLE device_sessions (
    id             UUID        PRIMARY KEY,
    user_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_name    TEXT        NOT NULL DEFAULT '',
    ip_address     TEXT        NOT NULL DEFAULT '',
    user_agent     TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at     TIMESTAMPTZ
);

-- ListActiveSessions: WHERE user_id = $1 AND revoked_at IS NULL ORDER BY last_active_at DESC
CREATE INDEX idx_device_sessions_active
    ON device_sessions (user_id, last_active_at DESC)
    WHERE revoked_at IS NULL;
