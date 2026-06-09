package clickhouse

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestClassifySQL(t *testing.T) {
	c := CH{}
	tests := []struct {
		sql string
		op  db.Op
		tbl string
	}{
		{"SELECT 1", db.OpSelect, ""},
		{"SELECT * FROM events", db.OpSelect, "events"},
		{"SELECT * FROM logs.events", db.OpSelect, "events"},
		{"INSERT INTO hits (id) VALUES (1)", db.OpInsert, "hits"},
		{"UPDATE t SET a=1", db.OpUpdate, "t"},
		{"DELETE FROM t WHERE id=1", db.OpDelete, "t"},
		{"TRUNCATE TABLE t", db.OpTruncate, "t"},
		{"ALTER TABLE t ADD COLUMN x Int32", db.OpDDL, "t"},
		{"CREATE TABLE t (id UInt64) ENGINE=MergeTree", db.OpForbidden, ""},
		{"DROP TABLE t", db.OpForbidden, ""},
		{"SHOW DATABASES", db.OpReadMeta, ""},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", db.OpSelect, ""},
	}
	for _, tc := range tests {
		got, _ := c.ClassifySQL(tc.sql)
		if got.Op != tc.op {
			t.Errorf("ClassifySQL(%q) op=%q want %q", tc.sql, got.Op, tc.op)
		}
		if tc.tbl != "" && got.Table != tc.tbl {
			t.Errorf("ClassifySQL(%q) table=%q want %q", tc.sql, got.Table, tc.tbl)
		}
	}
}
