CREATE TABLE connections (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  host         TEXT NOT NULL,
  port         INTEGER NOT NULL DEFAULT 3306,
  username     TEXT NOT NULL,
  password_enc BLOB NOT NULL,
  default_db   TEXT,
  tls          TEXT NOT NULL DEFAULT 'disabled',
  color        TEXT,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(user_id, name)
);
CREATE INDEX idx_connections_user ON connections(user_id);
