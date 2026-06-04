package mysql

import (
	"context"
	"database/sql"
)

// ListDatabases returns database names. When includeSystem is false, MySQL/system
// schemas (mysql, information_schema, performance_schema, sys) are excluded.
//
// We use SHOW DATABASES rather than information_schema.schemata so Vitess/PlanetScale
// returns all keyspaces (which appear as databases) instead of only the current one.
func ListDatabases(ctx context.Context, db *sql.DB, includeSystem bool) ([]string, error) {
	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	excluded := map[string]bool{
		"mysql": true, "information_schema": true,
		"performance_schema": true, "sys": true,
	}
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !includeSystem && excluded[name] {
			continue
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

type TableInfo struct {
	Name    string `json:"name"`
	RowsEst int64  `json:"rows_est"`
	SizeMB  int64  `json:"size_mb"`
}

// ListSchemaColumns returns a map of table_name -> [column_name, ...] for all tables in the schema.
func ListSchemaColumns(ctx context.Context, db *sql.DB, schema string) (map[string][]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT table_name, column_name
		 FROM information_schema.columns
		 WHERE table_schema = ?
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

func ListTables(ctx context.Context, db *sql.DB, schema string) ([]TableInfo, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT table_name,
		        COALESCE(table_rows, 0),
		        COALESCE(ROUND((data_length + index_length) / 1024 / 1024), 0)
		 FROM information_schema.tables
		 WHERE table_schema = ?
		 ORDER BY table_name`,
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.RowsEst, &t.SizeMB); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type RowsPage struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Total   int64    `json:"total"`
	Page    int      `json:"page"`
	PerPage int      `json:"per_page"`
}

// Filter represents a single WHERE clause condition.
type Filter struct {
	Column   string `json:"column"`
	Operator string `json:"operator"` // =, <>, <, >, <=, >=, IN, NOT IN, IS NULL, IS NOT NULL, BETWEEN, NOT BETWEEN, LIKE, Contains, Not contains, Has prefix, Has suffix
	Value    string `json:"value"`
}

type RowsOpts struct {
	Schema  string
	Table   string
	Page    int      // 1-based
	PerPage int      // capped at 500
	SortCol string   // empty = no order
	SortDir string   // "asc" | "desc"
	Filters []Filter // optional WHERE filters
}

// buildWhereClause converts filters to a SQL WHERE clause and arguments.
// Returns the WHERE string (including " WHERE ..." or empty) and the args slice.
func buildWhereClause(filters []Filter) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}
	var conds []string
	var args []any
	for _, f := range filters {
		if f.Column == "" {
			continue
		}
		col := QuoteIdent(f.Column)
		switch f.Operator {
		case "=", "<>", "<", ">", "<=", ">=":
			conds = append(conds, col+" "+f.Operator+" ?")
			args = append(args, f.Value)
		case "LIKE":
			conds = append(conds, col+" LIKE ?")
			args = append(args, f.Value)
		case "Contains":
			conds = append(conds, col+" LIKE ?")
			args = append(args, "%"+f.Value+"%")
		case "Not contains":
			conds = append(conds, col+" NOT LIKE ?")
			args = append(args, "%"+f.Value+"%")
		case "Has prefix":
			conds = append(conds, col+" LIKE ?")
			args = append(args, f.Value+"%")
		case "Has suffix":
			conds = append(conds, col+" LIKE ?")
			args = append(args, "%"+f.Value)
		case "IS NULL":
			conds = append(conds, col+" IS NULL")
		case "IS NOT NULL":
			conds = append(conds, col+" IS NOT NULL")
		case "IN", "NOT IN":
			// Value is comma-separated; split and bind each.
			parts := splitCSV(f.Value)
			if len(parts) == 0 {
				continue
			}
			placeholders := make([]string, len(parts))
			for i, p := range parts {
				placeholders[i] = "?"
				args = append(args, p)
			}
			op := "IN"
			if f.Operator == "NOT IN" {
				op = "NOT IN"
			}
			conds = append(conds, col+" "+op+" ("+joinStrs(placeholders, ", ")+")")
		case "BETWEEN", "NOT BETWEEN":
			parts := splitCSV(f.Value)
			if len(parts) != 2 {
				continue
			}
			op := "BETWEEN"
			if f.Operator == "NOT BETWEEN" {
				op = "NOT BETWEEN"
			}
			conds = append(conds, col+" "+op+" ? AND ?")
			args = append(args, parts[0], parts[1])
		}
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + joinStrs(conds, " AND "), args
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ',' {
			out = append(out, trimSpace(cur))
			cur = ""
		} else {
			cur += string(c)
		}
	}
	if cur != "" {
		out = append(out, trimSpace(cur))
	}
	return out
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func joinStrs(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += sep + parts[i]
	}
	return out
}

func FetchTableRows(ctx context.Context, db *sql.DB, o RowsOpts) (RowsPage, error) {
	if o.Page < 1 {
		o.Page = 1
	}
	if o.PerPage < 1 {
		o.PerPage = 50
	}
	if o.PerPage > 500 {
		o.PerPage = 500
	}
	offset := (o.Page - 1) * o.PerPage

	schema := QuoteIdent(o.Schema)
	table := QuoteIdent(o.Table)
	qualified := schema + "." + table

	whereClause, whereArgs := buildWhereClause(o.Filters)

	var total int64
	countArgs := append([]any{}, whereArgs...)
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+qualified+whereClause, countArgs...).Scan(&total); err != nil {
		return RowsPage{}, err
	}

	orderBy := ""
	if o.SortCol != "" {
		dir := "ASC"
		if o.SortDir == "desc" {
			dir = "DESC"
		}
		orderBy = " ORDER BY " + QuoteIdent(o.SortCol) + " " + dir
	}

	queryArgs := append([]any{}, whereArgs...)
	queryArgs = append(queryArgs, o.PerPage, offset)
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+qualified+whereClause+orderBy+" LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return RowsPage{}, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return RowsPage{}, err
	}
	page := RowsPage{Columns: cols, Total: total, Page: o.Page, PerPage: o.PerPage}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return RowsPage{}, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		page.Rows = append(page.Rows, vals)
	}
	return page, rows.Err()
}
