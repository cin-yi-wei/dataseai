-- Phase 2 password reset: a per-user email address plus short-lived,
-- single-use reset codes (the code itself is never stored — only its hash).
ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS password_reset_codes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL,
  code_hash  TEXT    NOT NULL,
  expires_at INTEGER NOT NULL, -- unix seconds
  used       INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_reset_codes_user ON password_reset_codes(user_id);
