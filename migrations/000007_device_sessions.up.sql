-- Таблица устройств (одна запись = одна залогиненная сессия пользователя).
-- Переживает ротацию refresh-токенов: последующие рефреши обновляют last_active_at,
-- но не создают новую запись об устройстве.
CREATE TABLE device_sessions (
    id             uuid        PRIMARY KEY,
    user_id        uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_name    text        NOT NULL DEFAULT '',
    ip_address     text        NOT NULL DEFAULT '',
    user_agent     text        NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_active_at timestamptz NOT NULL DEFAULT now(),
    revoked_at     timestamptz NULL
);

CREATE INDEX idx_device_sessions_user_id
    ON device_sessions (user_id);

CREATE INDEX idx_device_sessions_active
    ON device_sessions (user_id, last_active_at DESC)
    WHERE revoked_at IS NULL;

-- Связываем каждый refresh-токен с устройством.
-- NOT NULL — безопасно: миграция 000006 сделала TRUNCATE CASCADE.
ALTER TABLE refresh_sessions
    ADD COLUMN device_session_id uuid NOT NULL
        REFERENCES device_sessions(id) ON DELETE CASCADE;

CREATE INDEX idx_refresh_sessions_device
    ON refresh_sessions (device_session_id);
