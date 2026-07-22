-- Custom currency rules: user-authored, modular currency definitions.
--
-- A rule is a small declarative document (window + filters + requirements)
-- stored as JSONB in `definition`. The API evaluates it against the user's
-- flights the same way the built-in regulatory engine does, but the shape of
-- the rule is user data rather than hardcoded regulation.
--
-- Sharing is opt-in: a rule with is_shared = true exposes a read-only preview
-- via its share_token, which another user can import as a copy into their own
-- account. imported_from records provenance (the source rule id) for imports.

CREATE TABLE custom_currency_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    description TEXT,
    emoji VARCHAR(16),
    definition JSONB NOT NULL,
    is_shared BOOLEAN NOT NULL DEFAULT FALSE,
    share_token VARCHAR(64),
    imported_from UUID REFERENCES custom_currency_rules(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One rule per share token (tokens are only set when sharing is enabled).
CREATE UNIQUE INDEX idx_custom_currency_rules_share_token
    ON custom_currency_rules(share_token)
    WHERE share_token IS NOT NULL;

CREATE INDEX idx_custom_currency_rules_user ON custom_currency_rules(user_id);

CREATE TRIGGER update_custom_currency_rules_updated_at
    BEFORE UPDATE ON custom_currency_rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
