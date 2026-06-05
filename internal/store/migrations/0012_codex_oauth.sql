-- Per-user Codex (ChatGPT subscription) OAuth bundle, stored encrypted.
ALTER TABLE users ADD COLUMN codex_token_enc BLOB;
ALTER TABLE users ADD COLUMN codex_refresh_enc BLOB;
ALTER TABLE users ADD COLUMN codex_expires_at_ms INTEGER NOT NULL DEFAULT 0;
