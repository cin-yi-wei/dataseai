package mysql

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/conray/dataseai/internal/db"
)

// isoDateTimeRE matches an ISO-8601 datetime prefix (the part MySQL DATETIME
// cannot ingest verbatim due to the `T` separator and optional `Z`/offset).
var isoDateTimeRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)

// coerceValue normalizes ISO-8601 datetime strings (e.g. those rendered by
// the grid as `2026-06-03T04:05:48.280689Z`) into MySQL's DATETIME literal
// form `YYYY-MM-DD HH:MM:SS[.ffffff]` so the driver accepts them under
// MySQL strict mode. Non-matching values pass through untouched.
func coerceValue(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	if !isoDateTimeRE.MatchString(s) {
		return v
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05.999999999", s)
			if err != nil {
				t, err = time.Parse("2006-01-02T15:04:05", s)
				if err != nil {
					return v
				}
			}
		}
	}
	t = t.UTC()
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02 15:04:05")
	}
	return t.Format("2006-01-02 15:04:05.999999")
}

func coerceValues(vs []any) []any {
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = coerceValue(v)
	}
	return out
}

func qualifiedName(schema, table string) string {
	q := MySQL{}.QuoteIdent
	if schema == "" {
		return q(table)
	}
	return q(schema) + "." + q(table)
}

func whereByPK(pkCols []string, pkVals []any) (string, []any) {
	q := MySQL{}.QuoteIdent
	parts := make([]string, len(pkCols))
	args := make([]any, len(pkCols))
	for i, col := range pkCols {
		parts[i] = q(col) + " = ?"
		args[i] = pkVals[i]
	}
	return strings.Join(parts, " AND "), args
}

// PrimaryKey returns the ordered primary-key column names for a table.
// MySQL uses information_schema; sqlite is supported as a lightweight test stub.
func (MySQL) PrimaryKey(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ? AND column_key = 'PRI'
		ORDER BY ordinal_position
	`, schema, table)
	if err == nil {
		defer rows.Close()
		var out []string
		for rows.Next() {
			var col string
			if err := rows.Scan(&col); err != nil {
				return nil, err
			}
			out = append(out, col)
		}
		return out, rows.Err()
	}
	if !strings.Contains(err.Error(), "no such table") {
		return nil, err
	}

	rows, err = sqlDB.QueryContext(ctx, "PRAGMA table_info("+MySQL{}.QuoteIdent(table)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var (
			cid     int
			name    string
			typ     string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		if pk > 0 {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

func (m MySQL) UpdateCell(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, args := whereByPK(pkCols, pkVals)
	res, err := sqlDB.ExecContext(
		ctx,
		"UPDATE "+qualifiedName(schema, table)+" SET "+m.QuoteIdent(col)+" = ? WHERE "+where,
		append([]any{coerceValue(newVal)}, args...)...,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (m MySQL) InsertRow(ctx context.Context, sqlDB *sql.DB, schema, table string, cols []string, vals []any) (int64, error) {
	if len(cols) == 0 || len(cols) != len(vals) {
		return 0, errors.New("cols/vals empty or mismatched")
	}
	quotedCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, col := range cols {
		quotedCols[i] = m.QuoteIdent(col)
		placeholders[i] = "?"
	}
	res, err := sqlDB.ExecContext(
		ctx,
		"INSERT INTO "+qualifiedName(schema, table)+" ("+strings.Join(quotedCols, ", ")+") VALUES ("+strings.Join(placeholders, ", ")+")",
		coerceValues(vals)...,
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (MySQL) DeleteRow(ctx context.Context, sqlDB *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, db.ErrNoPrimaryKey
	}
	where, args := whereByPK(pkCols, pkVals)
	res, err := sqlDB.ExecContext(ctx, "DELETE FROM "+qualifiedName(schema, table)+" WHERE "+where, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// --- 無主鍵表：以「整列所有欄位值」比對來鎖定單一列（TablePlus/DBeaver 同法）。
// NULL 欄位用 IS NULL 比對。送 UPDATE/DELETE 前先 COUNT，剛好 1 列才動手，
// 0 列或 >1 列一律中止，避免誤改／誤刪（尤其有完全相同的重複列時）。

// whereByMatch 產生 WHERE：非 NULL 用 `col = ?`，NULL 用 `col IS NULL`。
func whereByMatch(cols []string, vals []any) (string, []any) {
	q := MySQL{}.QuoteIdent
	parts := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols))
	for i, c := range cols {
		if vals[i] == nil {
			parts = append(parts, q(c)+" IS NULL")
			continue
		}
		parts = append(parts, q(c)+" = ?")
		args = append(args, coerceValue(vals[i]))
	}
	return strings.Join(parts, " AND "), args
}

// matchGuard 確認比對條件剛好鎖定 1 列，否則回錯（不動資料）。
func matchGuard(ctx context.Context, sqlDB *sql.DB, schema, table, where string, args []any) error {
	var n int64
	if err := sqlDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM "+qualifiedName(schema, table)+" WHERE "+where, args...,
	).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return errors.New("找不到符合的列（資料可能已被其他人變動）")
	}
	if n > 1 {
		return db.ErrAmbiguousRow
	}
	return nil
}

func (m MySQL) UpdateCellByMatch(ctx context.Context, sqlDB *sql.DB, schema, table string, matchCols []string, matchVals []any, col string, newVal any) (int64, error) {
	if len(matchCols) == 0 || len(matchCols) != len(matchVals) {
		return 0, errors.New("no match columns")
	}
	where, args := whereByMatch(matchCols, matchVals)
	if err := matchGuard(ctx, sqlDB, schema, table, where, args); err != nil {
		return 0, err
	}
	res, err := sqlDB.ExecContext(ctx,
		"UPDATE "+qualifiedName(schema, table)+" SET "+m.QuoteIdent(col)+" = ? WHERE "+where,
		append([]any{coerceValue(newVal)}, args...)...,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (m MySQL) DeleteRowByMatch(ctx context.Context, sqlDB *sql.DB, schema, table string, matchCols []string, matchVals []any) (int64, error) {
	if len(matchCols) == 0 || len(matchCols) != len(matchVals) {
		return 0, errors.New("no match columns")
	}
	where, args := whereByMatch(matchCols, matchVals)
	if err := matchGuard(ctx, sqlDB, schema, table, where, args); err != nil {
		return 0, err
	}
	res, err := sqlDB.ExecContext(ctx, "DELETE FROM "+qualifiedName(schema, table)+" WHERE "+where, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
