CREATE TABLE query_history (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  connection_id INTEGER NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
  database_name TEXT,
  sql_text      TEXT NOT NULL,
  duration_ms   INTEGER,
  rows_affected INTEGER,
  error_message TEXT,
  source        TEXT NOT NULL DEFAULT 'user',
  executed_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_history_user_time ON query_history(user_id, executed_at DESC);
