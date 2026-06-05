-- DataGrid (DML) write permissions: same schema as AI writes but a separate
-- scope so a (user, conn, db, table) can carry distinct rules for AI vs DML.
-- 0009 created ai_write_policy with UNIQUE(user, conn, db, table). We need
-- to widen that to include scope. SQLite can't drop table-level uniques
-- in-place, so recreate the table.
ALTER TABLE ai_write_policy RENAME TO ai_write_policy_old;

CREATE TABLE ai_write_policy (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL,
  connection_id   INTEGER NOT NULL,
  database_name   TEXT    NOT NULL,
  table_name      TEXT    NOT NULL,
  scope           TEXT    NOT NULL DEFAULT 'ai',
  allow_insert    INTEGER NOT NULL DEFAULT 0,
  allow_update    INTEGER NOT NULL DEFAULT 0,
  allow_delete    INTEGER NOT NULL DEFAULT 0,
  allow_ddl       INTEGER NOT NULL DEFAULT 0,
  updated_at      DATETIME NOT NULL,
  UNIQUE(user_id, connection_id, database_name, table_name, scope),
  FOREIGN KEY (user_id)       REFERENCES users(id)       ON DELETE CASCADE,
  FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
);

INSERT INTO ai_write_policy (id, user_id, connection_id, database_name, table_name, scope,
    allow_insert, allow_update, allow_delete, allow_ddl, updated_at)
SELECT id, user_id, connection_id, database_name, table_name, 'ai',
    allow_insert, allow_update, allow_delete, allow_ddl, updated_at
  FROM ai_write_policy_old;

DROP TABLE ai_write_policy_old;

CREATE INDEX idx_ai_write_policy_user_conn ON ai_write_policy(user_id, connection_id);

-- Per-user master switch for DataGrid (manual row edit/insert/delete) writes.
-- AI uses ai_writes_enabled; this is its DataGrid counterpart so users can
-- enable one without the other.
ALTER TABLE users ADD COLUMN dml_writes_enabled INTEGER NOT NULL DEFAULT 0;
