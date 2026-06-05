-- Per-user Claude Code OAuth token (paste from ~/.claude/.credentials.json
-- after running `claude login` locally). Stored encrypted with the server
-- master key just like the LLM API keys.
ALTER TABLE users ADD COLUMN claudecode_token_enc BLOB;
