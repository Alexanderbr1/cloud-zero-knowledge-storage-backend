-- Метаданные объектов в MinIO; MIME-тип на сервере не храним.
-- encrypted_file_key / file_iv — обязательны: шифрование на клиенте является требованием системы.
CREATE TABLE stored_blobs (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  file_name text NOT NULL,
  object_key text NOT NULL UNIQUE,
  upload_method text NOT NULL,
  encrypted_file_key bytea NOT NULL,
  file_iv bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_stored_blobs_user_id ON stored_blobs(user_id);
