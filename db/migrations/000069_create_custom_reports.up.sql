-- Custom reports: a saved flight filter plus a grouping and a metric,
-- rendered on the Reports page and exportable on their own.

CREATE TABLE custom_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(120) NOT NULL,
    definition JSONB NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_custom_reports_user ON custom_reports(user_id, position);

CREATE TRIGGER update_custom_reports_updated_at
    BEFORE UPDATE ON custom_reports
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
