CREATE TABLE sessions (
  token         TEXT PRIMARY KEY,
  user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
  last_used_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  user_agent    TEXT,
  expires_at    DATETIME NOT NULL
);
CREATE INDEX idx_sessions_user ON sessions(user_id);
