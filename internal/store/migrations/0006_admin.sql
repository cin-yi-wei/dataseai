-- Add is_admin column to users.
ALTER TABLE users ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0;

-- First user becomes admin.
UPDATE users SET is_admin = 1 WHERE id = (SELECT MIN(id) FROM users);
