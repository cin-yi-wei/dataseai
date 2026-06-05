package store

import (
	"database/sql"
	"time"
)

type AIAuditRow struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	ConnectionID   int64     `json:"connection_id"`
	Database       string    `json:"database_name"`
	Table          string    `json:"table_name"`
	Operation      string    `json:"operation"`
	SQL            string    `json:"sql_text"`
	Status         string    `json:"status"` // proposed|executed|denied|cancelled|failed
	Scope          string    `json:"scope"`  // ai|dml
	RowsAffected   *int64    `json:"rows_affected"`
	ErrorMessage   string    `json:"error_message"`
	ExplainSummary string    `json:"explain_summary"`
	CreatedAt      time.Time `json:"created_at"`
}

// WriteAIAudit inserts an audit row. If r.Scope is empty, defaults to "ai"
// for backward compatibility with the original AI-only call sites.
func (s *Store) WriteAIAudit(r AIAuditRow) (int64, error) {
	if r.Scope == "" {
		r.Scope = string(ScopeAI)
	}
	now := time.Now().UTC()
	res, err := s.DB.Exec(`
        INSERT INTO ai_write_audit
            (user_id, connection_id, database_name, table_name, operation,
             sql_text, status, rows_affected, error_message, explain_summary, scope, created_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.UserID, r.ConnectionID, r.Database, r.Table, r.Operation,
		r.SQL, r.Status, r.RowsAffected, r.ErrorMessage, r.ExplainSummary, r.Scope, now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateAIAuditStatus(id int64, status string, rowsAffected *int64, errMsg string) error {
	_, err := s.DB.Exec(
		`UPDATE ai_write_audit SET status=?, rows_affected=?, error_message=? WHERE id=?`,
		status, rowsAffected, errMsg, id,
	)
	return err
}

// RecentAIAudit returns the latest audit rows for a user across all scopes.
// Kept for backward compat — RecentAuditByScope filters to one scope.
func (s *Store) RecentAIAudit(userID int64, limit int) ([]AIAuditRow, error) {
	return s.recentAudit(userID, "", limit)
}

// RecentAuditByScope filters audit rows to one scope (ai|dml).
func (s *Store) RecentAuditByScope(userID int64, scope PolicyScope, limit int) ([]AIAuditRow, error) {
	return s.recentAudit(userID, string(scope), limit)
}

func (s *Store) recentAudit(userID int64, scope string, limit int) ([]AIAuditRow, error) {
	if limit <= 0 {
		limit = 50
	} else if limit > 500 {
		limit = 500
	}
	args := []any{userID}
	q := `
        SELECT id, user_id, connection_id, database_name, table_name, operation,
               sql_text, status, rows_affected, COALESCE(error_message,''),
               COALESCE(explain_summary,''), COALESCE(scope,'ai'), created_at
          FROM ai_write_audit
         WHERE user_id=?`
	if scope != "" {
		q += ` AND scope=?`
		args = append(args, scope)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AIAuditRow
	for rows.Next() {
		var r AIAuditRow
		var rowsAff sql.NullInt64
		var created string
		if err := rows.Scan(&r.ID, &r.UserID, &r.ConnectionID, &r.Database, &r.Table,
			&r.Operation, &r.SQL, &r.Status, &rowsAff, &r.ErrorMessage,
			&r.ExplainSummary, &r.Scope, &created); err != nil {
			return nil, err
		}
		if rowsAff.Valid {
			v := rowsAff.Int64
			r.RowsAffected = &v
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, r)
	}
	return out, rows.Err()
}
