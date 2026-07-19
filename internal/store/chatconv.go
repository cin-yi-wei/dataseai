package store

import (
	"encoding/json"
	"time"
)

// Conversation is one persisted chat room, scoped to (user, connection, db).
type Conversation struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ConnID    int64  `json:"conn_id"`
	DBName    string `json:"db_name"`
	UpdatedAt int64  `json:"updated_at"`
}

// StoredChatMessage is one message's role + raw block JSON as persisted.
type StoredChatMessage struct {
	Role   string          `json:"role"`
	Blocks json.RawMessage `json:"blocks"`
}

// ListConversations returns the user's conversations for a (connection, db)
// scope, most-recently-updated first.
func (s *Store) ListConversations(userID, connID int64, dbName string) ([]Conversation, error) {
	rows, err := s.DB.Query(
		`SELECT id, name, conn_id, db_name, updated_at FROM chat_conversations
		 WHERE user_id=? AND conn_id=? AND db_name=? ORDER BY updated_at DESC, id DESC`,
		userID, connID, dbName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Name, &c.ConnID, &c.DBName, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateConversation makes a new empty conversation in the given scope.
func (s *Store) CreateConversation(userID, connID int64, dbName, name string, now time.Time) (Conversation, error) {
	ts := now.Unix()
	res, err := s.DB.Exec(
		`INSERT INTO chat_conversations(user_id, conn_id, db_name, name, created_at, updated_at)
		 VALUES(?,?,?,?,?,?)`,
		userID, connID, dbName, name, ts, ts)
	if err != nil {
		return Conversation{}, err
	}
	id, _ := res.LastInsertId()
	return Conversation{ID: id, Name: name, ConnID: connID, DBName: dbName, UpdatedAt: ts}, nil
}

// ownsConversation reports whether convID belongs to userID.
func (s *Store) ownsConversation(userID, convID int64) bool {
	var one int
	err := s.DB.QueryRow("SELECT 1 FROM chat_conversations WHERE id=? AND user_id=?", convID, userID).Scan(&one)
	return err == nil
}

// RenameConversation renames a conversation the user owns.
func (s *Store) RenameConversation(userID, convID int64, name string) error {
	res, err := s.DB.Exec(
		"UPDATE chat_conversations SET name=?, updated_at=updated_at WHERE id=? AND user_id=?",
		name, convID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteConversation removes a conversation (and its messages) the user owns.
func (s *Store) DeleteConversation(userID, convID int64) error {
	if !s.ownsConversation(userID, convID) {
		return ErrNotFound
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM chat_messages WHERE conversation_id=?", convID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec("DELETE FROM chat_conversations WHERE id=?", convID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// GetMessages returns a conversation's messages in order (empty if not owned).
func (s *Store) GetMessages(userID, convID int64) ([]StoredChatMessage, error) {
	if !s.ownsConversation(userID, convID) {
		return nil, ErrNotFound
	}
	rows, err := s.DB.Query(
		"SELECT role, blocks FROM chat_messages WHERE conversation_id=? ORDER BY seq", convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredChatMessage
	for rows.Next() {
		var m StoredChatMessage
		var blocks string
		if err := rows.Scan(&m.Role, &blocks); err != nil {
			return nil, err
		}
		m.Blocks = json.RawMessage(blocks)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReplaceMessages overwrites a conversation's messages with the given list and
// bumps updated_at. Simple whole-transcript save (chats are small).
func (s *Store) ReplaceMessages(userID, convID int64, msgs []StoredChatMessage, now time.Time) error {
	if !s.ownsConversation(userID, convID) {
		return ErrNotFound
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM chat_messages WHERE conversation_id=?", convID); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO chat_messages(conversation_id, seq, role, blocks, created_at) VALUES(?,?,?,?,?)")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	ts := now.Unix()
	for i, m := range msgs {
		blocks := string(m.Blocks)
		if blocks == "" {
			blocks = "[]"
		}
		if _, err := stmt.Exec(convID, i, m.Role, blocks, ts); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	_ = stmt.Close()
	if _, err := tx.Exec("UPDATE chat_conversations SET updated_at=? WHERE id=?", ts, convID); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
