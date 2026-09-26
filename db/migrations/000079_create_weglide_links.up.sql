-- A pilot's link to their WeGlide account: one row per user holding the
-- personal WeGlide API key, encrypted with BACKUP_CREDENTIALS_KEY
-- (AES-256-GCM, 12-byte nonce prepended to the ciphertext). The key is never
-- returned by the API and is not part of the JSON export.
-- requests_today/requests_day count calls against WeGlide's 60 requests per
-- key per UTC day. last_sync_at is the end of the last complete sync and the
-- cursor for the next one; last_sync_status/last_sync_error describe the
-- latest run.

CREATE TABLE weglide_links (
    user_id           UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    api_key_encrypted BYTEA NOT NULL,
    weglide_user_id   TEXT NULL CHECK (weglide_user_id IS NULL OR char_length(weglide_user_id) <= 64),
    last_sync_at      TIMESTAMPTZ NULL,
    last_sync_status  TEXT NULL CHECK (last_sync_status IS NULL OR last_sync_status IN ('ok', 'partial', 'failed')),
    last_sync_error   TEXT NULL CHECK (last_sync_error IS NULL OR char_length(last_sync_error) <= 500),
    requests_today    INTEGER NOT NULL DEFAULT 0 CHECK (requests_today >= 0),
    requests_day      DATE NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER update_weglide_links_updated_at
    BEFORE UPDATE ON weglide_links
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE weglide_links IS 'WeGlide account link: encrypted personal API key, sync state and daily request budget';
COMMENT ON COLUMN weglide_links.api_key_encrypted IS 'nonce || AES-256-GCM ciphertext of the WeGlide API key';
