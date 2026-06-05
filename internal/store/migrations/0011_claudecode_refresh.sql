-- Claude Code OAuth: store the refresh token + expiry so we can transparently
-- refresh the access token without asking the user to redo the auth dance.
ALTER TABLE users ADD COLUMN claudecode_refresh_enc BLOB;
ALTER TABLE users ADD COLUMN claudecode_expires_at_ms INTEGER NOT NULL DEFAULT 0;
