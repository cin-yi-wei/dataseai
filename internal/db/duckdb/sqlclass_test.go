package duckdb

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestClassifySQL(t *testing.T) {
	d := DuckDB{}
	tests := []struct {
		sql  string
		op   db.Op
		tbl  string
	}{
		{"SELECT 1", db.OpSelect, ""},
		{"SELECT * FROM users", db.OpSelect, "users"},
		{"SELECT * FROM app.users WHERE id=1", db.OpSelect, "users"},
		{"INSERT INTO orders (id) VALUES (1)", db.OpInsert, "orders"},
		{"UPDATE tbl SET a=1", db.OpUpdate, "tbl"},
		{"DELETE FROM tbl WHERE id=1", db.OpDelete, "tbl"},
		{"CREATE TABLE t (id INT)", db.OpForbidden, ""},
		{"DROP TABLE t", db.OpForbidden, ""},
		{"ALTER TABLE t ADD COLUMN x INT", db.OpDDL, "t"},
		{"PRAGMA database_list", db.OpReadMeta, ""},
	}
	for _, tc := range tests {
		c, err := d.ClassifySQL(tc.sql)
		if err != nil && tc.op != db.OpUnknown {
			t.Errorf("ClassifySQL(%q) err=%v", tc.sql, err)
			continue
		}
		if c.Op != tc.op {
			t.Errorf("ClassifySQL(%q) op=%q want %q", tc.sql, c.Op, tc.op)
		}
		if tc.tbl != "" && c.Table != tc.tbl {
			t.Errorf("ClassifySQL(%q) table=%q want %q", tc.sql, c.Table, tc.tbl)
		}
	}
}
