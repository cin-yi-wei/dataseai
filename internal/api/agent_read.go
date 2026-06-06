package api

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/conray/dataseai/internal/mysql"
)

func listDatabasesViaExecutor(ctx context.Context, exec mysql.Executor, includeSystem bool) ([]string, error) {
	out, err := exec.Run(ctx, "SHOW DATABASES", mysql.RunOpts{})
	if err != nil {
		return nil, err
	}
	excluded := map[string]bool{
		"mysql": true, "information_schema": true, "performance_schema": true, "sys": true,
	}
	var names []string
	for _, row := range out.Rows {
		if len(row) == 0 {
			continue
		}
		name := fmt.Sprint(row[0])
		if !includeSystem && excluded[name] {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func listTablesViaExecutor(ctx context.Context, exec mysql.Executor, schema string) ([]mysql.TableInfo, error) {
	sql := `SELECT table_name,
		        COALESCE(table_rows, 0),
		        COALESCE(ROUND((data_length + index_length) / 1024 / 1024), 0)
		 FROM information_schema.tables
		 WHERE table_schema = ` + sqlString(schema) + `
		 ORDER BY table_name`
	out, err := exec.Run(ctx, sql, mysql.RunOpts{})
	if err != nil {
		return nil, err
	}
	tables := make([]mysql.TableInfo, 0, len(out.Rows))
	for _, row := range out.Rows {
		if len(row) < 3 {
			continue
		}
		tables = append(tables, mysql.TableInfo{
			Name:    fmt.Sprint(row[0]),
			RowsEst: anyInt64(row[1]),
			SizeMB:  anyInt64(row[2]),
		})
	}
	return tables, nil
}

func listSchemaColumnsViaExecutor(ctx context.Context, exec mysql.Executor, schema string) (map[string][]string, error) {
	sql := `SELECT table_name, column_name
		 FROM information_schema.columns
		 WHERE table_schema = ` + sqlString(schema) + `
		 ORDER BY table_name, ordinal_position`
	out, err := exec.Run(ctx, sql, mysql.RunOpts{})
	if err != nil {
		return nil, err
	}
	tables := make(map[string][]string)
	for _, row := range out.Rows {
		if len(row) < 2 {
			continue
		}
		table := fmt.Sprint(row[0])
		col := fmt.Sprint(row[1])
		tables[table] = append(tables[table], col)
	}
	return tables, nil
}

func fetchTableRowsViaExecutor(ctx context.Context, exec mysql.Executor, o mysql.RowsOpts) (mysql.RowsPage, error) {
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
	qualified := mysql.QuoteIdent(o.Schema) + "." + mysql.QuoteIdent(o.Table)
	whereSQL := buildWhereSQL(o.Filters)
	countSQL := "SELECT COUNT(*) FROM " + qualified + whereSQL
	countOut, err := exec.Run(ctx, countSQL, mysql.RunOpts{})
	if err != nil {
		return mysql.RowsPage{}, err
	}
	var total int64
	if len(countOut.Rows) > 0 && len(countOut.Rows[0]) > 0 {
		total = anyInt64(countOut.Rows[0][0])
	}
	orderBy := ""
	if o.SortCol != "" {
		dir := "ASC"
		if o.SortDir == "desc" {
			dir = "DESC"
		}
		orderBy = " ORDER BY " + mysql.QuoteIdent(o.SortCol) + " " + dir
	}
	rowsSQL := "SELECT * FROM " + qualified + whereSQL + orderBy + " LIMIT " + strconv.Itoa(o.PerPage) + " OFFSET " + strconv.Itoa(offset)
	rowsOut, err := exec.Run(ctx, rowsSQL, mysql.RunOpts{MaxRows: o.PerPage})
	if err != nil {
		return mysql.RowsPage{}, err
	}
	return mysql.RowsPage{
		Columns: rowsOut.Columns,
		Rows:    rowsOut.Rows,
		Total:   total,
		Page:    o.Page,
		PerPage: o.PerPage,
	}, nil
}

func buildWhereSQL(filters []mysql.Filter) string {
	if len(filters) == 0 {
		return ""
	}
	var conds []string
	for _, f := range filters {
		if f.Column == "" {
			continue
		}
		col := mysql.QuoteIdent(f.Column)
		switch f.Operator {
		case "=", "<>", "<", ">", "<=", ">=":
			conds = append(conds, col+" "+f.Operator+" "+sqlString(f.Value))
		case "LIKE":
			conds = append(conds, col+" LIKE "+sqlString(f.Value))
		case "Contains":
			conds = append(conds, col+" LIKE "+sqlString("%"+f.Value+"%"))
		case "Not contains":
			conds = append(conds, col+" NOT LIKE "+sqlString("%"+f.Value+"%"))
		case "Has prefix":
			conds = append(conds, col+" LIKE "+sqlString(f.Value+"%"))
		case "Has suffix":
			conds = append(conds, col+" LIKE "+sqlString("%"+f.Value))
		case "IS NULL":
			conds = append(conds, col+" IS NULL")
		case "IS NOT NULL":
			conds = append(conds, col+" IS NOT NULL")
		case "IN", "NOT IN":
			parts := splitFilterCSV(f.Value)
			if len(parts) == 0 {
				continue
			}
			vals := make([]string, 0, len(parts))
			for _, p := range parts {
				vals = append(vals, sqlString(p))
			}
			op := "IN"
			if f.Operator == "NOT IN" {
				op = "NOT IN"
			}
			conds = append(conds, col+" "+op+" ("+strings.Join(vals, ", ")+")")
		case "BETWEEN", "NOT BETWEEN":
			parts := splitFilterCSV(f.Value)
			if len(parts) != 2 {
				continue
			}
			op := "BETWEEN"
			if f.Operator == "NOT BETWEEN" {
				op = "NOT BETWEEN"
			}
			conds = append(conds, col+" "+op+" "+sqlString(parts[0])+" AND "+sqlString(parts[1]))
		}
	}
	if len(conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conds, " AND ")
}

func splitFilterCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func describeTableViaExecutor(ctx context.Context, exec mysql.Executor, schema, table string) (mysql.Structure, error) {
	sql := `SELECT column_name, column_type, is_nullable, IFNULL(column_default,''),
		        IFNULL(extra,''), IFNULL(column_comment,''), IFNULL(column_key,'')
		 FROM information_schema.columns
		 WHERE table_schema = ` + sqlString(schema) + `
		   AND table_name = ` + sqlString(table) + `
		 ORDER BY ordinal_position`
	out, err := exec.Run(ctx, sql, mysql.RunOpts{})
	if err != nil {
		return mysql.Structure{}, err
	}
	var structure mysql.Structure
	for _, row := range out.Rows {
		if len(row) < 7 {
			continue
		}
		nullable := fmt.Sprint(row[2])
		structure.Columns = append(structure.Columns, mysql.Column{
			Name:     fmt.Sprint(row[0]),
			Type:     fmt.Sprint(row[1]),
			Nullable: nullable == "YES",
			Default:  fmt.Sprint(row[3]),
			Extra:    fmt.Sprint(row[4]),
			Comment:  fmt.Sprint(row[5]),
			Key:      fmt.Sprint(row[6]),
		})
	}
	createOut, err := exec.Run(ctx, "SHOW CREATE TABLE "+mysql.QuoteIdent(schema)+"."+mysql.QuoteIdent(table), mysql.RunOpts{})
	if err != nil {
		return structure, err
	}
	if len(createOut.Rows) > 0 && len(createOut.Rows[0]) > 1 {
		structure.CreateSQL = fmt.Sprint(createOut.Rows[0][1])
	}
	return structure, nil
}

func listIndexesViaExecutor(ctx context.Context, exec mysql.Executor, schema, table string) ([]mysql.Index, error) {
	sql := `SELECT index_name, column_name, non_unique, IFNULL(index_type,'BTREE')
		 FROM information_schema.statistics
		 WHERE table_schema = ` + sqlString(schema) + `
		   AND table_name = ` + sqlString(table) + `
		 ORDER BY index_name, seq_in_index`
	out, err := exec.Run(ctx, sql, mysql.RunOpts{})
	if err != nil {
		return nil, err
	}
	type acc struct {
		Name      string
		Columns   []string
		NonUnique int64
		IndexType string
	}
	var ordered []*acc
	byName := map[string]*acc{}
	for _, row := range out.Rows {
		if len(row) < 4 {
			continue
		}
		name := fmt.Sprint(row[0])
		entry, ok := byName[name]
		if !ok {
			entry = &acc{Name: name, NonUnique: anyInt64(row[2]), IndexType: fmt.Sprint(row[3])}
			byName[name] = entry
			ordered = append(ordered, entry)
		}
		entry.Columns = append(entry.Columns, fmt.Sprint(row[1]))
	}
	indexes := make([]mysql.Index, 0, len(ordered))
	for _, entry := range ordered {
		indexes = append(indexes, mysql.Index{
			Name:      entry.Name,
			Columns:   entry.Columns,
			Unique:    entry.NonUnique == 0,
			IndexType: entry.IndexType,
		})
	}
	return indexes, nil
}

func listForeignKeysViaExecutor(ctx context.Context, exec mysql.Executor, schema, table string) ([]mysql.ForeignKey, error) {
	sql := `SELECT k.constraint_name, k.column_name,
		        k.referenced_table_name, k.referenced_column_name,
		        IFNULL(c.delete_rule,''), IFNULL(c.update_rule,'')
		 FROM information_schema.key_column_usage k
		 LEFT JOIN information_schema.referential_constraints c
		   ON c.constraint_schema = k.constraint_schema
		  AND c.constraint_name = k.constraint_name
		 WHERE k.table_schema = ` + sqlString(schema) + `
		   AND k.table_name = ` + sqlString(table) + `
		   AND k.referenced_table_name IS NOT NULL
		 ORDER BY k.constraint_name, k.ordinal_position`
	out, err := exec.Run(ctx, sql, mysql.RunOpts{})
	if err != nil {
		return nil, err
	}
	fks := make([]mysql.ForeignKey, 0, len(out.Rows))
	for _, row := range out.Rows {
		if len(row) < 6 {
			continue
		}
		fks = append(fks, mysql.ForeignKey{
			Name:      fmt.Sprint(row[0]),
			Column:    fmt.Sprint(row[1]),
			RefTable:  fmt.Sprint(row[2]),
			RefColumn: fmt.Sprint(row[3]),
			OnDelete:  fmt.Sprint(row[4]),
			OnUpdate:  fmt.Sprint(row[5]),
		})
	}
	return fks, nil
}

func primaryKeyViaExecutor(ctx context.Context, exec mysql.Executor, schema, table string) ([]string, error) {
	sql := `SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = ` + sqlString(schema) + ` AND table_name = ` + sqlString(table) + ` AND column_key = 'PRI'
		ORDER BY ordinal_position`
	out, err := exec.Run(ctx, sql, mysql.RunOpts{})
	if err != nil {
		return nil, err
	}
	cols := make([]string, 0, len(out.Rows))
	for _, row := range out.Rows {
		if len(row) > 0 {
			cols = append(cols, fmt.Sprint(row[0]))
		}
	}
	return cols, nil
}

func updateCellViaExecutor(ctx context.Context, exec mysql.Executor, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, mysql.ErrNoPrimaryKey
	}
	where := whereByPKSQL(pkCols, pkVals)
	sql := "UPDATE " + mysql.QuoteIdent(schema) + "." + mysql.QuoteIdent(table) +
		" SET " + mysql.QuoteIdent(col) + " = " + sqlLiteral(newVal) + " WHERE " + where
	out, err := exec.Run(ctx, sql, mysql.RunOpts{})
	if err != nil {
		return 0, err
	}
	return out.RowsAffected, nil
}

func insertRowViaExecutor(ctx context.Context, exec mysql.Executor, schema, table string, values map[string]any) (id int64, affected int64, err error) {
	if len(values) == 0 {
		return 0, 0, fmt.Errorf("values required")
	}
	cols := make([]string, 0, len(values))
	for col := range values {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	quotedCols := make([]string, 0, len(cols))
	literals := make([]string, 0, len(cols))
	for _, col := range cols {
		quotedCols = append(quotedCols, mysql.QuoteIdent(col))
		literals = append(literals, sqlLiteral(values[col]))
	}
	sql := "INSERT INTO " + mysql.QuoteIdent(schema) + "." + mysql.QuoteIdent(table) +
		" (" + strings.Join(quotedCols, ", ") + ") VALUES (" + strings.Join(literals, ", ") + ")"
	out, err := exec.Run(ctx, sql, mysql.RunOpts{})
	if err != nil {
		return 0, 0, err
	}
	return 0, out.RowsAffected, nil
}

func deleteRowViaExecutor(ctx context.Context, exec mysql.Executor, schema, table string, pkCols []string, pkVals []any) (int64, error) {
	if len(pkCols) == 0 || len(pkCols) != len(pkVals) {
		return 0, mysql.ErrNoPrimaryKey
	}
	sql := "DELETE FROM " + mysql.QuoteIdent(schema) + "." + mysql.QuoteIdent(table) + " WHERE " + whereByPKSQL(pkCols, pkVals)
	out, err := exec.Run(ctx, sql, mysql.RunOpts{})
	if err != nil {
		return 0, err
	}
	return out.RowsAffected, nil
}

func whereByPKSQL(pkCols []string, pkVals []any) string {
	parts := make([]string, 0, len(pkCols))
	for i, col := range pkCols {
		parts = append(parts, mysql.QuoteIdent(col)+" = "+sqlLiteral(pkVals[i]))
	}
	return strings.Join(parts, " AND ")
}

func sqlLiteral(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if x {
			return "1"
		}
		return "0"
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return sqlString(x)
	default:
		return sqlString(fmt.Sprint(v))
	}
}

func anyInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case jsonNumber:
		i, _ := strconv.ParseInt(string(n), 10, 64)
		return i
	case []byte:
		i, _ := strconv.ParseInt(string(n), 10, 64)
		return i
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}

type jsonNumber string

func sqlString(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += "''"
			continue
		}
		out += string(r)
	}
	return out + "'"
}
