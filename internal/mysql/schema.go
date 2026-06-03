package mysql

import (
	"context"
	"database/sql"
)

type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default"`
	Extra    string `json:"extra"`
	Comment  string `json:"comment"`
	Key      string `json:"key"` // "PRI", "UNI", "MUL", or ""
}

type Structure struct {
	Columns   []Column `json:"columns"`
	CreateSQL string   `json:"create_sql"`
}

type Index struct {
	Name      string   `json:"name"`
	Columns   []string `json:"columns"`
	Unique    bool     `json:"unique"`
	IndexType string   `json:"index_type"`
}

type ForeignKey struct {
	Name      string `json:"name"`
	Column    string `json:"column"`
	RefTable  string `json:"ref_table"`
	RefColumn string `json:"ref_column"`
	OnDelete  string `json:"on_delete"`
	OnUpdate  string `json:"on_update"`
}

func DescribeTable(ctx context.Context, db *sql.DB, schema, table string) (Structure, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT column_name, column_type, is_nullable, IFNULL(column_default,''),
		        IFNULL(extra,''), IFNULL(column_comment,''), IFNULL(column_key,'')
		 FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY ordinal_position`,
		schema, table,
	)
	if err != nil {
		return Structure{}, err
	}
	var out Structure
	for rows.Next() {
		var c Column
		var nullable string
		if err := rows.Scan(&c.Name, &c.Type, &nullable, &c.Default, &c.Extra, &c.Comment, &c.Key); err != nil {
			rows.Close()
			return Structure{}, err
		}
		c.Nullable = nullable == "YES"
		out.Columns = append(out.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Structure{}, err
	}

	// SHOW CREATE TABLE quotes its identifiers — schema/table must be safe.
	qualified := QuoteIdent(schema) + "." + QuoteIdent(table)
	var tbl, createSQL string
	if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE "+qualified).Scan(&tbl, &createSQL); err != nil {
		return out, err
	}
	out.CreateSQL = createSQL
	return out, nil
}

func ListIndexes(ctx context.Context, db *sql.DB, schema, table string) ([]Index, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT index_name, column_name, non_unique, IFNULL(index_type,'BTREE')
		 FROM information_schema.statistics
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY index_name, seq_in_index`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Group rows by index_name into ordered column lists.
	type acc struct {
		Name      string
		Columns   []string
		NonUnique int
		IndexType string
	}
	var ordered []*acc
	byName := map[string]*acc{}
	for rows.Next() {
		var iname, cname, idxType string
		var nonUnique int
		if err := rows.Scan(&iname, &cname, &nonUnique, &idxType); err != nil {
			return nil, err
		}
		entry, ok := byName[iname]
		if !ok {
			entry = &acc{Name: iname, NonUnique: nonUnique, IndexType: idxType}
			byName[iname] = entry
			ordered = append(ordered, entry)
		}
		entry.Columns = append(entry.Columns, cname)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Index, 0, len(ordered))
	for _, e := range ordered {
		out = append(out, Index{
			Name: e.Name, Columns: e.Columns,
			Unique: e.NonUnique == 0, IndexType: e.IndexType,
		})
	}
	return out, nil
}

func ListForeignKeys(ctx context.Context, db *sql.DB, schema, table string) ([]ForeignKey, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT k.constraint_name, k.column_name,
		        k.referenced_table_name, k.referenced_column_name,
		        IFNULL(c.delete_rule,''), IFNULL(c.update_rule,'')
		 FROM information_schema.key_column_usage k
		 LEFT JOIN information_schema.referential_constraints c
		   ON c.constraint_schema = k.constraint_schema
		  AND c.constraint_name = k.constraint_name
		 WHERE k.table_schema = ? AND k.table_name = ?
		   AND k.referenced_table_name IS NOT NULL
		 ORDER BY k.constraint_name, k.ordinal_position`,
		schema, table,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ForeignKey
	for rows.Next() {
		var fk ForeignKey
		if err := rows.Scan(&fk.Name, &fk.Column, &fk.RefTable, &fk.RefColumn, &fk.OnDelete, &fk.OnUpdate); err != nil {
			return nil, err
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}
