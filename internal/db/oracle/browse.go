package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

func (o Oracle) FetchTableRows(ctx context.Context, sqlDB *sql.DB, opts db.RowsOpts) (db.RowsPage, error) {
	if opts.Page < 1 {
		opts.Page = 1
	}
	if opts.PerPage < 1 {
		opts.PerPage = 50
	}
	if opts.PerPage > db.MaxRowsPerPage {
		opts.PerPage = db.MaxRowsPerPage
	}
	offset := (opts.Page - 1) * opts.PerPage

	qualified := qualifiedName(o, opts.Schema, opts.Table)
	whereClause, whereArgs := buildWhereClause(o, opts.Filters)

	var total int64
	countArgs := append([]any{}, whereArgs...)
	if err := sqlDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+qualified+whereClause, countArgs...).Scan(&total); err != nil {
		return db.RowsPage{}, err
	}

	orderBy := ""
	if opts.SortCol != "" {
		dir := "ASC"
		if strings.EqualFold(opts.SortDir, "desc") {
			dir = "DESC"
		}
		orderBy = " ORDER BY " + o.QuoteIdent(opts.SortCol) + " " + dir
	}

	// Oracle 12c+ OFFSET/FETCH syntax
	queryArgs := append([]any{}, whereArgs...)
	queryArgs = append(queryArgs, offset, opts.PerPage)
	// Placeholders for offset/limit come after where args
	offsetPH := o.Placeholder(len(whereArgs) + 1)
	fetchPH := o.Placeholder(len(whereArgs) + 2)

	query := fmt.Sprintf("SELECT * FROM %s%s%s OFFSET %s ROWS FETCH NEXT %s ROWS ONLY",
		qualified, whereClause, orderBy, offsetPH, fetchPH)

	rows, err := sqlDB.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return db.RowsPage{}, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return db.RowsPage{}, err
	}
	page := db.RowsPage{Columns: cols, Total: total, Page: opts.Page, PerPage: opts.PerPage}
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

func buildWhereClause(o Oracle, filters []db.Filter) (string, []any) {
	if len(filters) == 0 {
		return "", nil
	}
	var conds []string
	var args []any
	n := 1
	for _, f := range filters {
		if f.Column == "" {
			continue
		}
		col := o.QuoteIdent(f.Column)
		switch f.Operator {
		case "=", "<>", "<", ">", "<=", ">=":
			conds = append(conds, col+" "+f.Operator+" "+o.Placeholder(n))
			args = append(args, f.Value)
			n++
		case "LIKE":
			conds = append(conds, col+" LIKE "+o.Placeholder(n))
			args = append(args, f.Value)
			n++
		case "Contains":
			conds = append(conds, col+" LIKE "+o.Placeholder(n))
			args = append(args, "%"+f.Value+"%")
			n++
		case "Not contains":
			conds = append(conds, col+" NOT LIKE "+o.Placeholder(n))
			args = append(args, "%"+f.Value+"%")
			n++
		case "Has prefix":
			conds = append(conds, col+" LIKE "+o.Placeholder(n))
			args = append(args, f.Value+"%")
			n++
		case "Has suffix":
			conds = append(conds, col+" LIKE "+o.Placeholder(n))
			args = append(args, "%"+f.Value)
			n++
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
				phs[i] = o.Placeholder(n)
				args = append(args, p)
				n++
			}
			conds = append(conds, col+" "+f.Operator+" ("+strings.Join(phs, ", ")+")")
		case "BETWEEN", "NOT BETWEEN":
			parts := splitCSV(f.Value)
			if len(parts) != 2 {
				continue
			}
			conds = append(conds, fmt.Sprintf("%s %s %s AND %s", col, f.Operator, o.Placeholder(n), o.Placeholder(n+1)))
			args = append(args, parts[0], parts[1])
			n += 2
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
