CREATE TABLE users (
  id uuid PRIMARY KEY,
  email text NOT NULL UNIQUE,
  password_hash text NOT NULL,
  crypto_salt bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
