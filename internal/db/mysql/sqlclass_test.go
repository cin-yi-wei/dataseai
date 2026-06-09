package mysql

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestClassifyBasic(t *testing.T) {
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
		{"insert-basic", "INSERT INTO t (a) VALUES (1)", db.OpInsert, "", "t", false, false},
		{"insert-low-priority", "INSERT LOW_PRIORITY INTO t VALUES(1)", db.OpInsert, "", "t", false, false},
		{"insert-ignore", "INSERT IGNORE INTO `db`.`t` VALUES(1)", db.OpInsert, "db", "t", false, false},
		{"insert-select", "INSERT INTO a SELECT * FROM b", db.OpInsert, "", "a", false, false},
		{"update", "UPDATE t SET a=1", db.OpUpdate, "", "t", false, false},
		{"update-ignore", "UPDATE IGNORE db.t SET a=1", db.OpUpdate, "db", "t", false, false},
		{"update-join", "UPDATE a JOIN b ON a.x=b.x SET a.y=b.y", db.OpUpdate, "", "a", false, false},
		{"update-comma", "UPDATE a, b SET a.y=b.y", db.OpUpdate, "", "a", false, false},
		{"delete", "DELETE FROM t WHERE id=1", db.OpDelete, "", "t", false, false},
		{"delete-quick", "DELETE LOW_PRIORITY QUICK IGNORE FROM `db`.`t`", db.OpDelete, "db", "t", false, false},
		{"truncate-bare", "TRUNCATE t", db.OpTruncate, "", "t", false, false},
		{"truncate-table", "TRUNCATE TABLE db.t", db.OpTruncate, "db", "t", false, false},
		{"alter", "ALTER TABLE t ADD COLUMN x INT", db.OpDDL, "", "t", false, false},
		{"rename-table", "RENAME TABLE a TO b", db.OpDDL, "", "a", false, false},
		{"create-forbidden", "CREATE TABLE t (id INT)", db.OpForbidden, "", "", false, false},
		{"drop-forbidden", "DROP TABLE t", db.OpForbidden, "", "", false, false},
		{"show", "SHOW TABLES", db.OpReadMeta, "", "", false, false},
		{"describe", "DESCRIBE t", db.OpReadMeta, "", "", false, false},
		{"desc", "DESC t", db.OpReadMeta, "", "", false, false},
		{"explain", "EXPLAIN SELECT * FROM t", db.OpReadMeta, "", "", false, false},
		{"multi", "SELECT 1; SELECT 2", db.OpSelect, "", "", true, false},
		{"multi-trailing-semicolon", "SELECT 1;", db.OpSelect, "", "", false, false},
		{"line-comment", "-- hi\nSELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"block-comment", "/* x */ SELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"backtick-with-escape", "SELECT * FROM `weird``name`", db.OpSelect, "", "weird`name", false, false},
		{"unknown", "DO SOMETHING", db.OpUnknown, "", "", false, false},
		{"empty", "   ", db.OpUnknown, "", "", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := MySQL{}.ClassifySQL(c.sql)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Op != c.wantOp {
				t.Errorf("op=%v want %v", got.Op, c.wantOp)
			}
			if got.DB != c.wantDB {
				t.Errorf("db=%q want %q", got.DB, c.wantDB)
			}
			if got.Table != c.wantTable {
				t.Errorf("table=%q want %q", got.Table, c.wantTable)
			}
			if got.Multi != c.wantMulti {
				t.Errorf("multi=%v want %v", got.Multi, c.wantMulti)
			}
		})
	}
}
