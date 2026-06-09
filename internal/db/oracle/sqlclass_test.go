package oracle

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestClassifySQL(t *testing.T) {
	o := Oracle{}
	tests := []struct {
		sql string
		op  db.Op
		tbl string
	}{
		{"SELECT 1 FROM dual", db.OpSelect, "DUAL"},
		{"SELECT * FROM employees", db.OpSelect, "EMPLOYEES"},
		{`SELECT * FROM "HR"."Employees"`, db.OpSelect, "Employees"},
		{"INSERT INTO orders (id) VALUES (1)", db.OpInsert, "ORDERS"},
		{"UPDATE emp SET sal=1000", db.OpUpdate, "EMP"},
		{"DELETE FROM emp WHERE id=1", db.OpDelete, "EMP"},
		{"MERGE INTO target USING src ON (target.id=src.id) WHEN MATCHED THEN UPDATE SET x=1", db.OpInsert, "TARGET"},
		{"ALTER TABLE emp ADD COLUMN x VARCHAR2(10)", db.OpDDL, "EMP"},
		{"CREATE TABLE t (id NUMBER)", db.OpForbidden, ""},
		{"DROP TABLE t", db.OpForbidden, ""},
		{"TRUNCATE TABLE t", db.OpForbidden, ""},
		{"WITH cte AS (SELECT 1 FROM dual) SELECT * FROM cte", db.OpSelect, ""},
	}
	for _, tc := range tests {
		got, _ := o.ClassifySQL(tc.sql)
		if got.Op != tc.op {
			t.Errorf("ClassifySQL(%q) op=%q want %q", tc.sql, got.Op, tc.op)
		}
		if tc.tbl != "" && got.Table != tc.tbl {
			t.Errorf("ClassifySQL(%q) table=%q want %q", tc.sql, got.Table, tc.tbl)
		}
	}
}
