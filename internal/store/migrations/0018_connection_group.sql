-- Optional grouping label for connections (e.g. "Production", "Local").
-- Empty string means ungrouped.
ALTER TABLE connections ADD COLUMN group_name TEXT NOT NULL DEFAULT '';
