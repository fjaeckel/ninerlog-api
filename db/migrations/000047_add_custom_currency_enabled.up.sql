-- Allow pausing a custom currency rule without deleting it. Disabled rules are
-- kept and listed, but not evaluated and not surfaced as active currency.
ALTER TABLE custom_currency_rules
    ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT TRUE;
