package snowflake

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestClassifySQL(t *testing.T) {
	s := Snowflake{}
	tests := []struct {
		sql string
		op  db.Op
		tbl string
	}{
		{"SELECT 1", db.OpSelect, ""},
		{"SELECT * FROM users", db.OpSelect, "users"},
		{`SELECT * FROM "My Schema"."Orders"`, db.OpSelect, "Orders"},
		{"INSERT INTO orders (id) VALUES (1)", db.OpInsert, "orders"},
		{"UPDATE tbl SET a=1", db.OpUpdate, "tbl"},
		{"DELETE FROM tbl WHERE id=1", db.OpDelete, "tbl"},
		{"MERGE INTO target USING src ON target.id=src.id WHEN MATCHED THEN UPDATE SET x=1", db.OpInsert, "target"},
		{"CREATE TABLE t (id INT)", db.OpForbidden, ""},
		{"DROP TABLE t", db.OpForbidden, ""},
		{"TRUNCATE TABLE t", db.OpForbidden, ""},
		{"ALTER TABLE t ADD COLUMN x INT", db.OpDDL, "t"},
		{"SHOW DATABASES", db.OpReadMeta, ""},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", db.OpSelect, ""},
	}
	for _, tc := range tests {
		c, _ := s.ClassifySQL(tc.sql)
		if c.Op != tc.op {
			t.Errorf("ClassifySQL(%q) op=%q want %q", tc.sql, c.Op, tc.op)
		}
		if tc.tbl != "" && c.Table != tc.tbl {
			t.Errorf("ClassifySQL(%q) table=%q want %q", tc.sql, c.Table, tc.tbl)
		}
	}
}
