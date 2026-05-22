DROP TABLE IF EXISTS password_reset_tokens;

ALTER TABLE users
    DROP COLUMN IF EXISTS kek_encrypted_master,
    DROP COLUMN IF EXISTS kek_encrypted_recovery,
    DROP COLUMN IF EXISTS recovery_salt;
