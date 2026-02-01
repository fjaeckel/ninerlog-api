-- Drop indexes
DROP INDEX IF EXISTS idx_password_reset_tokens_expires_at;
DROP INDEX IF EXISTS idx_password_reset_tokens_token_hash;
DROP INDEX IF EXISTS idx_password_reset_tokens_user_id;

-- Drop password_reset_tokens table
DROP TABLE IF EXISTS password_reset_tokens;
