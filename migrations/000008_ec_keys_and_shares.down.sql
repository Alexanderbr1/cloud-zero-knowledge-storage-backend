DROP TABLE IF EXISTS file_shares;

ALTER TABLE users
    DROP COLUMN IF EXISTS encrypted_private_key,
    DROP COLUMN IF EXISTS public_key;
