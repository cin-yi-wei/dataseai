package store

import (
	"database/sql"
	"time"
)

type AIAuditRow struct {
	ID             int64
	UserID         int64
	ConnectionID   int64
	Database       string
	Table          string
	Operation      string
	SQL            string
	Status         string // proposed|executed|denied|cancelled|failed
	RowsAffected   *int64
	ErrorMessage   string
	ExplainSummary string
	CreatedAt      time.Time
}

func (s *Store) WriteAIAudit(r AIAuditRow) (int64, error) {
	now := time.Now().UTC()
	res, err := s.DB.Exec(`
        INSERT INTO ai_write_audit
            (user_id, connection_id, database_name, table_name, operation,
             sql_text, status, rows_affected, error_message, explain_summary, created_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		r.UserID, r.ConnectionID, r.Database, r.Table, r.Operation,
		r.SQL, r.Status, r.RowsAffected, r.ErrorMessage, r.ExplainSummary, now.Format(time.RFC3339Nano),
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

func (s *Store) RecentAIAudit(userID int64, limit int) ([]AIAuditRow, error) {
	if limit <= 0 {
		limit = 50
	} else if limit > 500 {
		limit = 500
	}
	rows, err := s.DB.Query(`
        SELECT id, user_id, connection_id, database_name, table_name, operation,
               sql_text, status, rows_affected, COALESCE(error_message,''),
               COALESCE(explain_summary,''), created_at
          FROM ai_write_audit
         WHERE user_id=?
         ORDER BY created_at DESC LIMIT ?`,
		userID, limit)
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
			&r.ExplainSummary, &created); err != nil {
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
