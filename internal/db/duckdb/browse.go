package duckdb

import (
	"context"
	"database/sql"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func buildWhereClause(d DuckDB, filters []db.Filter) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}
	var conds []string
	var args []any
	for _, f := range filters {
		if f.Column == "" {
			continue
		}
		col := d.QuoteIdent(f.Column)
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
			parts := splitCSV(f.Value)
			if len(parts) == 0 {
				continue
			}
			phs := make([]string, len(parts))
			for i, p := range parts {
				phs[i] = "?"
				args = append(args, p)
			}
			conds = append(conds, col+" "+f.Operator+" ("+strings.Join(phs, ", ")+")")
		case "BETWEEN", "NOT BETWEEN":
			parts := splitCSV(f.Value)
			if len(parts) != 2 {
				continue
			}
			conds = append(conds, col+" "+f.Operator+" ? AND ?")
			args = append(args, parts[0], parts[1])
		}
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (d DuckDB) FetchTableRows(ctx context.Context, sqlDB *sql.DB, o db.RowsOpts) (db.RowsPage, error) {
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

	qualified := qualifiedName(d, o.Schema, o.Table)
	whereClause, whereArgs := buildWhereClause(d, o.Filters)

	var total int64
	countArgs := append([]any{}, whereArgs...)
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+qualified+whereClause, countArgs...).Scan(&total); err != nil {
		return db.RowsPage{}, err
	}

	orderBy := ""
	if o.SortCol != "" {
		dir := "ASC"
		if o.SortDir == "desc" {
			dir = "DESC"
		}
		orderBy = " ORDER BY " + d.QuoteIdent(o.SortCol) + " " + dir
	}

	queryArgs := append([]any{}, whereArgs...)
	queryArgs = append(queryArgs, o.PerPage, offset)
	rows, err := sqlDB.QueryContext(ctx,
		"SELECT * FROM "+qualified+whereClause+orderBy+" LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		return db.RowsPage{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return db.RowsPage{}, err
	}
	page := db.RowsPage{Columns: cols, Total: total, Page: o.Page, PerPage: o.PerPage}
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
	return page, rows.Err()
}
