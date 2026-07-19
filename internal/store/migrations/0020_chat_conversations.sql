-- Persistent AI chat "rooms": conversations scoped per (user, connection, db),
-- and their messages (each message's blocks stored as JSON).
CREATE TABLE IF NOT EXISTS chat_conversations (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL,
  conn_id    INTEGER NOT NULL,
  db_name    TEXT    NOT NULL DEFAULT '',
  name       TEXT    NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chat_conv_scope
  ON chat_conversations(user_id, conn_id, db_name, updated_at);

CREATE TABLE IF NOT EXISTS chat_messages (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  conversation_id INTEGER NOT NULL,
  seq             INTEGER NOT NULL,
  role            TEXT    NOT NULL,
  blocks          TEXT    NOT NULL, -- JSON array of chat blocks
  created_at      INTEGER NOT NULL,
  FOREIGN KEY(conversation_id) REFERENCES chat_conversations(id)
);
CREATE INDEX IF NOT EXISTS idx_chat_msg_conv ON chat_messages(conversation_id, seq);
