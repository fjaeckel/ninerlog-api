-- Drop the NOT NULL on flight_imports.license_id.
--
-- Migration 13 created the column when an import was going to be scoped to one
-- licence's logbook. That design was not built: nothing writes the column and
-- nothing reads it. The INSERT in internal/repository/postgres/flight_import.go
-- has never listed it, so every history write since migration 13 has been
-- rejected by the constraint.
--
-- The write is best-effort -- ConfirmImport logs the failure and still returns
-- 201 -- so imports succeeded while GET /imports stayed empty, totalImports
-- stayed 0, and importsByFormat had nothing to group.
--
-- The column, its foreign key and its index are kept: they cost nothing on a
-- NULL column and the licence-scoped logbook may yet be built.

ALTER TABLE flight_imports ALTER COLUMN license_id DROP NOT NULL;
