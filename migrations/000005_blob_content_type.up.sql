-- MIME-тип объекта: обязателен в POST /storage/presign; для уже существующих строк задаём безопасный дефолт.
ALTER TABLE stored_blobs
    ADD COLUMN content_type text NOT NULL DEFAULT 'application/octet-stream';

ALTER TABLE stored_blobs
    ALTER COLUMN content_type DROP DEFAULT;
