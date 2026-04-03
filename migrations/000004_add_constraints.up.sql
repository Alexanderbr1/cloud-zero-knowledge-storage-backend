-- Ограничение на допустимые методы загрузки (defence-in-depth; приложение уже передаёт только 'presigned_put').
ALTER TABLE stored_blobs
    ADD CONSTRAINT chk_stored_blobs_upload_method
    CHECK (upload_method IN ('presigned_put'));

-- Ограничение на длину email (RFC 5321: max 320 символов; мин. 3 = a@b).
-- Приложение валидирует на уровне HTTP, это вторая линия защиты на уровне БД.
ALTER TABLE users
    ADD CONSTRAINT chk_users_email_length
    CHECK (char_length(email) BETWEEN 3 AND 320);

-- Индекс для периодической очистки просроченных, но не отозванных сессий.
-- Запрос: DELETE FROM refresh_sessions WHERE expires_at < now() AND revoked_at IS NULL
CREATE INDEX idx_refresh_sessions_expires_at
    ON refresh_sessions (expires_at)
    WHERE revoked_at IS NULL;
