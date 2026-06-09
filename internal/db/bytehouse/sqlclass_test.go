package bytehouse

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestClassifySQL(t *testing.T) {
	cases := []struct {
		name      string
		sql       string
		wantOp    db.Op
		wantDB    string
		wantTable string
		wantMulti bool
		wantErr   bool
	}{
		{"select", "SELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"select-no-from", "SELECT 1", db.OpSelect, "", "", false, false},
		{"insert", "INSERT INTO t (a) VALUES (1)", db.OpInsert, "", "t", false, false},
		{"insert-schema", "INSERT INTO `db`.`t` VALUES(1)", db.OpInsert, "db", "t", false, false},
		{"update", "UPDATE t SET a=1", db.OpUpdate, "", "t", false, false},
		{"update-schema", "UPDATE `db`.`t` SET a=1", db.OpUpdate, "db", "t", false, false},
		{"delete", "DELETE FROM t WHERE id=1", db.OpDelete, "", "t", false, false},
		{"truncate", "TRUNCATE TABLE t", db.OpTruncate, "", "t", false, false},
		{"alter", "ALTER TABLE t ADD COLUMN x Int32", db.OpDDL, "", "t", false, false},
		{"create-forbidden", "CREATE TABLE t (id UInt64) ENGINE=MergeTree()", db.OpForbidden, "", "", false, false},
		{"drop-forbidden", "DROP TABLE t", db.OpForbidden, "", "", false, false},
		{"attach-forbidden", "ATTACH TABLE t", db.OpForbidden, "", "", false, false},
		{"show-readmeta", "SHOW TABLES", db.OpReadMeta, "", "", false, false},
		{"describe-readmeta", "DESCRIBE TABLE t", db.OpReadMeta, "", "", false, false},
		{"optimize-readmeta", "OPTIMIZE TABLE t FINAL", db.OpReadMeta, "", "", false, false},
		{"multi", "SELECT 1; SELECT 2", db.OpSelect, "", "", true, false},
		{"multi-trailing", "SELECT 1;", db.OpSelect, "", "", false, false},
		{"line-comment", "-- hi\nSELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"block-comment", "/* x */ SELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"backtick-schema-table", "SELECT * FROM `mydb`.`users`", db.OpSelect, "mydb", "users", false, false},
		{"backtick-escaped", "SELECT * FROM `weird``name`", db.OpSelect, "", "weird`name", false, false},
		{"with-cte", "WITH cte AS (SELECT 1) SELECT * FROM cte", db.OpSelect, "", "", false, false},
		{"empty", "   ", db.OpUnknown, "", "", false, true},
	}

	d := BH{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.ClassifySQL(tc.sql)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Op != tc.wantOp {
				t.Errorf("Op = %q, want %q", got.Op, tc.wantOp)
			}
			if got.DB != tc.wantDB {
				t.Errorf("DB = %q, want %q", got.DB, tc.wantDB)
			}
			if got.Table != tc.wantTable {
				t.Errorf("Table = %q, want %q", got.Table, tc.wantTable)
			}
			if got.Multi != tc.wantMulti {
				t.Errorf("Multi = %v, want %v", got.Multi, tc.wantMulti)
			}
		})
	}
}
