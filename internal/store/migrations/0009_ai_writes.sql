-- AI chat write permissions: per-user master switch + per-table policy + audit log.
ALTER TABLE users ADD COLUMN ai_writes_enabled INTEGER NOT NULL DEFAULT 0;

CREATE TABLE ai_write_policy (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL,
  connection_id   INTEGER NOT NULL,
  database_name   TEXT    NOT NULL,
  table_name      TEXT    NOT NULL,
  allow_insert    INTEGER NOT NULL DEFAULT 0,
  allow_update    INTEGER NOT NULL DEFAULT 0,
  allow_delete    INTEGER NOT NULL DEFAULT 0,
  allow_ddl       INTEGER NOT NULL DEFAULT 0,
  updated_at      DATETIME NOT NULL,
  UNIQUE(user_id, connection_id, database_name, table_name),
  FOREIGN KEY (user_id)       REFERENCES users(id)       ON DELETE CASCADE,
  FOREIGN KEY (connection_id) REFERENCES connections(id) ON DELETE CASCADE
);

CREATE INDEX idx_ai_write_policy_user_conn ON ai_write_policy(user_id, connection_id);

CREATE TABLE ai_write_audit (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL,
  connection_id   INTEGER NOT NULL,
  database_name   TEXT    NOT NULL,
  table_name      TEXT    NOT NULL,
  operation       TEXT    NOT NULL,
  sql_text        TEXT    NOT NULL,
  status          TEXT    NOT NULL,
  rows_affected   INTEGER,
  error_message   TEXT,
  explain_summary TEXT,
  created_at      DATETIME NOT NULL,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_ai_write_audit_user ON ai_write_audit(user_id, created_at DESC);
