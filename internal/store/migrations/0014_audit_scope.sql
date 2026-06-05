-- Add scope to the audit log so AI vs DataGrid (DML) writes can be queried
-- separately. Existing rows default to 'ai' (the only writer until now).
ALTER TABLE ai_write_audit ADD COLUMN scope TEXT NOT NULL DEFAULT 'ai';
CREATE INDEX idx_ai_write_audit_user_scope ON ai_write_audit(user_id, scope, created_at DESC);
