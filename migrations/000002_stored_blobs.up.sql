-- Метаданные blob'ов.
CREATE TABLE IF NOT EXISTS stored_blobs (
  id uuid PRIMARY KEY,
  object_key text NOT NULL UNIQUE,
  content_type text NULL,
  upload_method text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
