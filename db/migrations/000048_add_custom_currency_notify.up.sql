-- Per-rule notification opt-in. When true (and the rule is enabled and the user
-- has email notifications on), the notification checker emails when the rule is
-- expiring or has lapsed. Defaults to false — notifications are opt-in per rule.
ALTER TABLE custom_currency_rules
    ADD COLUMN notify BOOLEAN NOT NULL DEFAULT FALSE;
