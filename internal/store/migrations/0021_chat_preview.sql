-- Last-message preview shown in the conversation directory (LINE-style rows).
ALTER TABLE chat_conversations ADD COLUMN preview TEXT NOT NULL DEFAULT '';
