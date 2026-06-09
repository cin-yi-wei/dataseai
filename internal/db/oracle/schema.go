package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// ListDatabases returns Oracle schemas (users) the current session can see.
// Filters out Oracle-maintained system schemas on 12c+; falls back to all_users.
func (o Oracle) ListDatabases(ctx context.Context, sqlDB *sql.DB, _ bool) ([]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT username FROM all_users
		 WHERE oracle_maintained = 'N'
		 ORDER BY username`)
	if err != nil {
		// oracle_maintained column absent on pre-12c — fall back
		rows, err = sqlDB.QueryContext(ctx, `SELECT username FROM all_users ORDER BY username`)
		if err != nil {
			return nil, err
		}
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

func (o Oracle) ListTables(ctx context.Context, sqlDB *sql.DB, schema string) ([]db.TableInfo, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT table_name, NVL(num_rows, 0) FROM all_tables
		 WHERE owner = :1
		 ORDER BY table_name`,
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.TableInfo
	for rows.Next() {
		var ti db.TableInfo
		if err := rows.Scan(&ti.Name, &ti.RowsEst); err != nil {
			return nil, err
		}
		out = append(out, ti)
	}
	return out, rows.Err()
}

func (o Oracle) ListSchemaColumns(ctx context.Context, sqlDB *sql.DB, schema string) (map[string][]string, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT table_name, column_name FROM all_tab_columns
		 WHERE owner = :1
		 ORDER BY table_name, column_id`,
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

func (o Oracle) DescribeTable(ctx context.Context, sqlDB *sql.DB, schema, table string) (db.Structure, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT column_name, data_type, nullable, data_default, data_length, data_precision, data_scale
		 FROM all_tab_columns
		 WHERE owner = :1 AND table_name = :2
		 ORDER BY column_id`,
		schema, table)
	if err != nil {
		return db.Structure{}, err
	}
	var out db.Structure
	for rows.Next() {
		var c db.Column
		var nullable string
		var dataDefault sql.NullString
		var dataLen sql.NullInt64
		var dataPrecision, dataScale sql.NullInt64
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &dataDefault, &dataLen, &dataPrecision, &dataScale); err != nil {
			rows.Close()
			return db.Structure{}, err
		}
		c.Nullable = nullable == "Y"
		if dataDefault.Valid {
			c.Default = strings.TrimSpace(dataDefault.String)
		}
		// Append precision/scale to type if present
		if dataPrecision.Valid && dataPrecision.Int64 > 0 {
			if dataScale.Valid && dataScale.Int64 > 0 {
				c.Type = fmt.Sprintf("%s(%d,%d)", c.Type, dataPrecision.Int64, dataScale.Int64)
			} else {
				c.Type = fmt.Sprintf("%s(%d)", c.Type, dataPrecision.Int64)
			}
		} else if dataLen.Valid && dataLen.Int64 > 0 &&
			!strings.HasPrefix(c.Type, "NUMBER") &&
			!strings.HasPrefix(c.Type, "DATE") &&
			!strings.HasPrefix(c.Type, "TIMESTAMP") {
			c.Type = fmt.Sprintf("%s(%d)", c.Type, dataLen.Int64)
		}
		out.Columns = append(out.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return db.Structure{}, err
	}

	// Try DBMS_METADATA for CREATE DDL; fall back to synthesis.
	var ddl string
	err = sqlDB.QueryRowContext(ctx,
		`SELECT DBMS_METADATA.GET_DDL('TABLE', :1, :2) FROM dual`,
		table, schema).Scan(&ddl)
	if err == nil {
		out.CreateSQL = ddl
	} else {
		out.CreateSQL = synthesizeCreateTable(out.Columns, schema, table)
	}
	return out, nil
}

func synthesizeCreateTable(cols []db.Column, schema, table string) string {
	o := Oracle{}
	var sb strings.Builder
	sb.WriteString("CREATE TABLE ")
	if schema != "" {
		sb.WriteString(o.QuoteIdent(schema))
		sb.WriteByte('.')
	}
	sb.WriteString(o.QuoteIdent(table))
	sb.WriteString(" (\n")
	for i, c := range cols {
		sb.WriteString("  ")
		sb.WriteString(o.QuoteIdent(c.Name))
		sb.WriteByte(' ')
		sb.WriteString(c.Type)
		if !c.Nullable {
			sb.WriteString(" NOT NULL")
		}
		if c.Default != "" {
			sb.WriteString(" DEFAULT ")
			sb.WriteString(c.Default)
		}
		if i < len(cols)-1 {
			sb.WriteByte(',')
		}
		sb.WriteByte('\n')
	}
	sb.WriteString(")")
	return sb.String()
}

func (o Oracle) ListIndexes(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.Index, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT i.index_name, i.uniqueness, ic.column_name
		 FROM all_indexes i
		 JOIN all_ind_columns ic
		   ON ic.index_owner = i.owner AND ic.index_name = i.index_name
		 WHERE i.owner = :1 AND i.table_name = :2
		 ORDER BY i.index_name, ic.column_position`,
		schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	indexMap := make(map[string]*db.Index)
	var order []string
	for rows.Next() {
		var idxName, uniq, col string
		if err := rows.Scan(&idxName, &uniq, &col); err != nil {
			return nil, err
		}
		if _, ok := indexMap[idxName]; !ok {
			indexMap[idxName] = &db.Index{Name: idxName, Unique: uniq == "UNIQUE"}
			order = append(order, idxName)
		}
		indexMap[idxName].Columns = append(indexMap[idxName].Columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]db.Index, 0, len(order))
	for _, name := range order {
		out = append(out, *indexMap[name])
	}
	return out, nil
}

func (o Oracle) ListForeignKeys(ctx context.Context, sqlDB *sql.DB, schema, table string) ([]db.ForeignKey, error) {
	rows, err := sqlDB.QueryContext(ctx,
		`SELECT c.constraint_name,
		        cc.column_name,
		        rc.owner AS ref_owner,
		        rc.table_name AS ref_table,
		        rcc.column_name AS ref_column,
		        c.delete_rule
		 FROM all_constraints c
		 JOIN all_cons_columns cc
		   ON cc.owner = c.owner AND cc.constraint_name = c.constraint_name
		 JOIN all_constraints rc
		   ON rc.owner = c.r_owner AND rc.constraint_name = c.r_constraint_name
		 JOIN all_cons_columns rcc
		   ON rcc.owner = rc.owner AND rcc.constraint_name = rc.constraint_name AND rcc.position = cc.position
		 WHERE c.constraint_type = 'R'
		   AND c.owner = :1 AND c.table_name = :2
		 ORDER BY c.constraint_name, cc.position`,
		schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []db.ForeignKey
	for rows.Next() {
		var fk db.ForeignKey
		var refOwner string
		if err := rows.Scan(&fk.Name, &fk.Column, &refOwner, &fk.RefTable, &fk.RefColumn, &fk.OnDelete); err != nil {
			return nil, err
		}
		if refOwner != schema {
			fk.RefTable = refOwner + "." + fk.RefTable
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}
