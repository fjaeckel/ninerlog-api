DROP INDEX IF EXISTS idx_refresh_tokens_user_last_used;
DROP INDEX IF EXISTS idx_refresh_tokens_user_session;

ALTER TABLE refresh_tokens
    DROP COLUMN IF EXISTS rotated_at,
    DROP COLUMN IF EXISTS revoked_at,
    DROP COLUMN IF EXISTS last_used_at,
    DROP COLUMN IF EXISTS ip_address,
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS device_label,
    DROP COLUMN IF EXISTS session_id;
