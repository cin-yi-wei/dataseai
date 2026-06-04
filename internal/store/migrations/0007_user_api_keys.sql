-- Per-user API keys for LLM providers. Stored encrypted with the server master key.
ALTER TABLE users ADD COLUMN anthropic_api_key_enc BLOB;
ALTER TABLE users ADD COLUMN openai_api_key_enc BLOB;
ALTER TABLE users ADD COLUMN gemini_api_key_enc BLOB;
