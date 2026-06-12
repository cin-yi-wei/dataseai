package pg

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/conray/dataseai/internal/db"
)

// ListDatabases returns PostgreSQL schema names, treating schemas as "databases"
// in the UI sidebar. System schemas are always excluded.
func (p PG) ListDatabases(ctx context.Context, sqlDB *sql.DB, _ bool) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT schema_name FROM information_schema.schemata
		 WHERE schema_name NOT IN (
		   'information_schema','pg_catalog','pg_toast',
		   'pg_temp_1','pg_toast_temp_1'
		 )
		 ORDER BY schema_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// ListTables returns all tables/views in the given schema.
func (p PG) ListTables(ctx context.Context, sqlDB *sql.DB, schema string) ([]db.TableInfo, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT c.relname, c.relkind, COALESCE(obj_description(c.oid, 'pg_class'), '')
		 FROM pg_class c
		 JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = $1 AND c.relkind IN ('r','v','m','f','p')
		 ORDER BY c.relname`,
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.TableInfo
	for rows.Next() {
		var name, kind, comment string
		if err := rows.Scan(&name, &kind, &comment); err != nil {
			return nil, err
		}
		_ = comment // TableInfo has no Comment field; kept for future use
		_ = kind
		out = append(out, db.TableInfo{Name: name})
	}
	return out, rows.Err()
}

// ListSchemaColumns returns a map of table_name -> [column_name, ...] for all
// tables in the given schema.
func (p PG) ListSchemaColumns(ctx context.Context, sqlDB *sql.DB, schema string) (map[string][]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT table_name, column_name
		 FROM information_schema.columns
		 WHERE table_schema = $1
		 ORDER BY table_name, ordinal_position`,
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string][]string)
	for rows.Next() {
		var t, c string
		if err := rows.Scan(&t, &c); err != nil {
			return nil, err
		}
		out[t] = append(out[t], c)
	}
	return out, rows.Err()
}

// FetchTableRows returns a paginated, optionally filtered and sorted slice of
// rows from schema.table, plus the total count.
func (p PG) FetchTableRows(ctx context.Context, sqlDB *sql.DB, o db.RowsOpts) (db.RowsPage, error) {
	if o.Page < 1 {
		o.Page = 1
	}
	if o.PerPage < 1 {
		o.PerPage = 50
	}
	if o.PerPage > db.MaxRowsPerPage {
		o.PerPage = db.MaxRowsPerPage
	}
	offset := (o.Page - 1) * o.PerPage

	qualified := p.QuoteIdent(o.Schema) + "." + p.QuoteIdent(o.Table)

	whereClause, whereArgs := buildPGWhereClause(o.Filters)

	orderBy := ""
	if o.SortCol != "" {
		dir := "ASC"
		if strings.EqualFold(o.SortDir, "desc") {
			dir = "DESC"
		}
		orderBy = " ORDER BY " + p.QuoteIdent(o.SortCol) + " " + dir
	}

	// $N positional placeholders — WHERE args already claim 1..len(whereArgs),
	// so LIMIT and OFFSET take the next two positions.
	n := len(whereArgs)
	limitPH := p.Placeholder(n + 1)
	offsetPH := p.Placeholder(n + 2)

	queryArgs := append([]any{}, whereArgs...)
	queryArgs = append(queryArgs, o.PerPage, offset)

	query := fmt.Sprintf("SELECT * FROM %s%s%s LIMIT %s OFFSET %s",
		qualified, whereClause, orderBy, limitPH, offsetPH)

	rows, err := sqlDB.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return db.RowsPage{}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return db.RowsPage{}, err
	}
	page := db.RowsPage{Columns: cols, Page: o.Page, PerPage: o.PerPage}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return db.RowsPage{}, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		page.Rows = append(page.Rows, vals)
	}
	if err := rows.Err(); err != nil {
		return db.RowsPage{}, err
	}
	rows.Close()

	// Total is best-effort. With no filter, use pg_class.reltuples (the
	// planner's row estimate — instant) instead of a full COUNT(*) scan that
	// can blow the request deadline on large tables. With a filter, count the
	// matching subset. -1 signals "unknown" to the UI.
	page.Total = -1
	countCtx, cancelCount := context.WithTimeout(ctx, 3*time.Second)
	var total int64
	if len(o.Filters) == 0 {
		if err := sqlDB.QueryRowContext(countCtx,
			"SELECT reltuples::bigint FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = $1 AND c.relname = $2",
			o.Schema, o.Table).Scan(&total); err == nil && total >= 0 {
			page.Total = total
		}
	} else {
		countArgs := append([]any{}, whereArgs...)
		if err := sqlDB.QueryRowContext(countCtx, "SELECT COUNT(*) FROM "+qualified+whereClause, countArgs...).Scan(&total); err == nil {
			page.Total = total
		}
	}
	cancelCount()
	return page, nil
}

// buildPGWhereClause converts a slice of filters into a SQL WHERE clause with
// PostgreSQL $N positional placeholders. Returns an empty string and nil args
// if filters is empty.
func buildPGWhereClause(filters []db.Filter) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}
	var conds []string
	var args []any
	p := PG{}
	for _, f := range filters {
		if f.Column == "" {
			continue
		}
		col := p.QuoteIdent(f.Column)
		n := len(args) + 1 // next placeholder index
		switch f.Operator {
		case "=", "<>", "<", ">", "<=", ">=":
			conds = append(conds, col+" "+f.Operator+" "+p.Placeholder(n))
			args = append(args, f.Value)
		case "LIKE":
			conds = append(conds, col+" LIKE "+p.Placeholder(n))
			args = append(args, f.Value)
		case "Contains":
			conds = append(conds, col+" LIKE "+p.Placeholder(n))
			args = append(args, "%"+f.Value+"%")
		case "Not contains":
			conds = append(conds, col+" NOT LIKE "+p.Placeholder(n))
			args = append(args, "%"+f.Value+"%")
		case "Has prefix":
			conds = append(conds, col+" LIKE "+p.Placeholder(n))
			args = append(args, f.Value+"%")
		case "Has suffix":
			conds = append(conds, col+" LIKE "+p.Placeholder(n))
			args = append(args, "%"+f.Value)
		case "IS NULL":
			conds = append(conds, col+" IS NULL")
		case "IS NOT NULL":
			conds = append(conds, col+" IS NOT NULL")
		case "IN", "NOT IN":
			parts := pgSplitCSV(f.Value)
			if len(parts) == 0 {
				continue
			}
			placeholders := make([]string, len(parts))
			for i, part := range parts {
				placeholders[i] = p.Placeholder(n + i)
				args = append(args, part)
			}
			op := "IN"
			if f.Operator == "NOT IN" {
				op = "NOT IN"
			}
			conds = append(conds, col+" "+op+" ("+strings.Join(placeholders, ", ")+")")
		case "BETWEEN", "NOT BETWEEN":
			parts := pgSplitCSV(f.Value)
			if len(parts) != 2 {
				continue
			}
			op := "BETWEEN"
			if f.Operator == "NOT BETWEEN" {
				op = "NOT BETWEEN"
			}
			conds = append(conds, fmt.Sprintf("%s %s %s AND %s",
				col, op, p.Placeholder(n), p.Placeholder(n+1)))
			args = append(args, parts[0], parts[1])
		}
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func pgSplitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
