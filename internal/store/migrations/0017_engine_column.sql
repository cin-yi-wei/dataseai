ALTER TABLE connections ADD COLUMN engine TEXT NOT NULL DEFAULT 'mysql';
UPDATE connections SET engine='mysql' WHERE engine IS NULL OR engine='';
CREATE INDEX idx_connections_engine ON connections(engine);
