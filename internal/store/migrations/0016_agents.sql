-- LAN connector agents. A user can register one or more agents (one per
-- machine that bridges to a private network). Each agent has a token that
-- only the connector knows; we store its SHA-256 hex digest so brokers can
-- look up by hashed token without ever seeing plaintext.
CREATE TABLE agents (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  token_hash   TEXT NOT NULL UNIQUE,
  last_seen_at DATETIME,
  last_ip      TEXT,
  last_os      TEXT,
  last_version TEXT,
  created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(user_id, name)
);
CREATE INDEX idx_agents_user ON agents(user_id);

-- Existing connections can optionally route through an agent instead of
-- being dialed directly by the dataseai server. NULL = direct connection
-- (unchanged behaviour).
ALTER TABLE connections ADD COLUMN via_agent_id INTEGER REFERENCES agents(id) ON DELETE SET NULL;
CREATE INDEX idx_connections_via_agent ON connections(via_agent_id);
