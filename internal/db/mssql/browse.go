package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/conray/dataseai/internal/db"
)

func (m MSSQL) FetchTableRows(ctx context.Context, sqlDB *sql.DB, o db.RowsOpts) (db.RowsPage, error) {
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

	qualified := qualifiedName(m, o.Schema, o.Table)

	whereClause, whereArgs := buildWhereClause(m, o.Filters)

	// MSSQL requires ORDER BY when using OFFSET/FETCH.
	orderBy := " ORDER BY (SELECT NULL)"
	if o.SortCol != "" {
		dir := "ASC"
		if strings.EqualFold(o.SortDir, "desc") {
			dir = "DESC"
		}
		orderBy = " ORDER BY " + m.QuoteIdent(o.SortCol) + " " + dir
	}

	n := len(whereArgs)
	offsetPH := m.Placeholder(n + 1)
	limitPH := m.Placeholder(n + 2)

	queryArgs := append([]any{}, whereArgs...)
	queryArgs = append(queryArgs, offset, o.PerPage)

	query := fmt.Sprintf("SELECT * FROM %s%s%s OFFSET %s ROWS FETCH NEXT %s ROWS ONLY",
		qualified, whereClause, orderBy, offsetPH, limitPH)

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

	// Total is best-effort. With no filter, use the partition stats row estimate
	// (instant) instead of a full COUNT(*) scan that can blow the deadline on
	// large tables. With a filter, count the matching subset. -1 = "unknown".
	page.Total = -1
	countCtx, cancelCount := context.WithTimeout(ctx, 3*time.Second)
	var total int64
	if len(o.Filters) == 0 {
		if err := sqlDB.QueryRowContext(countCtx,
			m.useDB(o.Schema)+"SELECT ISNULL(SUM(row_count),0) FROM sys.dm_db_partition_stats WHERE object_id = OBJECT_ID(@p1) AND index_id IN (0,1)",
			defaultSchema+"."+o.Table).Scan(&total); err == nil {
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

func buildWhereClause(m MSSQL, filters []db.Filter) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}
	var conds []string
	var args []any
	for _, f := range filters {
		if f.Column == "" {
			continue
		}
		col := m.QuoteIdent(f.Column)
		n := len(args) + 1
		switch f.Operator {
		case "=", "<>", "<", ">", "<=", ">=":
			conds = append(conds, col+" "+f.Operator+" "+m.Placeholder(n))
			args = append(args, f.Value)
		case "LIKE":
			conds = append(conds, col+" LIKE "+m.Placeholder(n))
			args = append(args, f.Value)
		case "Contains":
			conds = append(conds, col+" LIKE "+m.Placeholder(n))
			args = append(args, "%"+f.Value+"%")
		case "Not contains":
			conds = append(conds, col+" NOT LIKE "+m.Placeholder(n))
			args = append(args, "%"+f.Value+"%")
		case "Has prefix":
			conds = append(conds, col+" LIKE "+m.Placeholder(n))
			args = append(args, f.Value+"%")
		case "Has suffix":
			conds = append(conds, col+" LIKE "+m.Placeholder(n))
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
			for i, part := range parts {
				phs[i] = m.Placeholder(n + i)
				args = append(args, part)
			}
			op := "IN"
			if f.Operator == "NOT IN" {
				op = "NOT IN"
			}
			conds = append(conds, col+" "+op+" ("+strings.Join(phs, ", ")+")")
		case "BETWEEN", "NOT BETWEEN":
			parts := splitCSV(f.Value)
			if len(parts) != 2 {
				continue
			}
			op := "BETWEEN"
			if f.Operator == "NOT BETWEEN" {
				op = "NOT BETWEEN"
			}
			conds = append(conds, fmt.Sprintf("%s %s %s AND %s",
				col, op, m.Placeholder(n), m.Placeholder(n+1)))
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
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
