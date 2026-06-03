package store

import (
	"database/sql"
	"time"
)

type HistoryInput struct {
	UserID       int64
	ConnectionID int64
	DatabaseName string
	SQLText      string
	DurationMs   int64
	RowsAffected int64
	ErrorMessage string
	Source       string // "user" | "ai"
}

type History struct {
	ID           int64
	UserID       int64
	ConnectionID int64
	DatabaseName string
	SQLText      string
	DurationMs   int64
	RowsAffected int64
	ErrorMessage string
	Source       string
	ExecutedAt   time.Time
}

// AddHistory persists a query history entry. No pruning. Use AddHistoryWithCap
// for retention enforcement.
func (s *Store) AddHistory(in HistoryInput) error {
	if in.Source == "" {
		in.Source = "user"
	}
	_, err := s.DB.Exec(
		`INSERT INTO query_history(user_id, connection_id, database_name, sql_text, duration_ms, rows_affected, error_message, source)
		 VALUES (?,?,?,?,?,?,?,?)`,
		in.UserID, in.ConnectionID, in.DatabaseName, in.SQLText,
		in.DurationMs, in.RowsAffected, in.ErrorMessage, in.Source,
	)
	return err
}

// AddHistoryWithCap inserts an entry then deletes anything beyond `max` per-user
// rows, keeping the newest. Cap <= 0 disables pruning.
func (s *Store) AddHistoryWithCap(in HistoryInput, max int) error {
	if err := s.AddHistory(in); err != nil {
		return err
	}
	if max <= 0 {
		return nil
	}
	_, err := s.DB.Exec(
		`DELETE FROM query_history
		 WHERE user_id = ?
		   AND id NOT IN (
		     SELECT id FROM query_history WHERE user_id = ?
		     ORDER BY executed_at DESC, id DESC
		     LIMIT ?
		   )`,
		in.UserID, in.UserID, max,
	)
	return err
}

func (s *Store) ListHistory(userID int64, limit, offset int) ([]History, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.DB.Query(
		`SELECT id, user_id, connection_id, database_name, sql_text,
		        duration_ms, rows_affected, error_message, source, executed_at
		 FROM query_history WHERE user_id = ?
		 ORDER BY executed_at DESC, id DESC LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []History
	for rows.Next() {
		var h History
		var dbName, errMsg sql.NullString
		if err := rows.Scan(&h.ID, &h.UserID, &h.ConnectionID, &dbName,
			&h.SQLText, &h.DurationMs, &h.RowsAffected, &errMsg,
			&h.Source, &h.ExecutedAt); err != nil {
			return nil, err
		}
		h.DatabaseName = dbName.String
		h.ErrorMessage = errMsg.String
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) DeleteHistoryEntry(userID, id int64) error {
	res, err := s.DB.Exec("DELETE FROM query_history WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ClearHistory(userID int64) error {
	_, err := s.DB.Exec("DELETE FROM query_history WHERE user_id = ?", userID)
	return err
}
