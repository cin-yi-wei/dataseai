package mysql

import "testing"

func TestClassifyBasic(t *testing.T) {
	cases := []struct {
		name      string
		sql       string
		wantOp    Op
		wantDB    string
		wantTable string
		wantMulti bool
		wantErr   bool
	}{
		{"select", "SELECT * FROM t", OpSelect, "", "t", false, false},
		{"select-no-from", "SELECT 1", OpSelect, "", "", false, false},
		{"insert-basic", "INSERT INTO t (a) VALUES (1)", OpInsert, "", "t", false, false},
		{"insert-low-priority", "INSERT LOW_PRIORITY INTO t VALUES(1)", OpInsert, "", "t", false, false},
		{"insert-ignore", "INSERT IGNORE INTO `db`.`t` VALUES(1)", OpInsert, "db", "t", false, false},
		{"insert-select", "INSERT INTO a SELECT * FROM b", OpInsert, "", "a", false, false},
		{"update", "UPDATE t SET a=1", OpUpdate, "", "t", false, false},
		{"update-ignore", "UPDATE IGNORE db.t SET a=1", OpUpdate, "db", "t", false, false},
		{"update-join", "UPDATE a JOIN b ON a.x=b.x SET a.y=b.y", OpUpdate, "", "a", false, false},
		{"update-comma", "UPDATE a, b SET a.y=b.y", OpUpdate, "", "a", false, false},
		{"delete", "DELETE FROM t WHERE id=1", OpDelete, "", "t", false, false},
		{"delete-quick", "DELETE LOW_PRIORITY QUICK IGNORE FROM `db`.`t`", OpDelete, "db", "t", false, false},
		{"truncate-bare", "TRUNCATE t", OpTruncate, "", "t", false, false},
		{"truncate-table", "TRUNCATE TABLE db.t", OpTruncate, "db", "t", false, false},
		{"alter", "ALTER TABLE t ADD COLUMN x INT", OpDDL, "", "t", false, false},
		{"rename-table", "RENAME TABLE a TO b", OpDDL, "", "a", false, false},
		{"create-forbidden", "CREATE TABLE t (id INT)", OpForbidden, "", "", false, false},
		{"drop-forbidden", "DROP TABLE t", OpForbidden, "", "", false, false},
		{"show", "SHOW TABLES", OpReadMeta, "", "", false, false},
		{"describe", "DESCRIBE t", OpReadMeta, "", "", false, false},
		{"desc", "DESC t", OpReadMeta, "", "", false, false},
		{"explain", "EXPLAIN SELECT * FROM t", OpReadMeta, "", "", false, false},
		{"multi", "SELECT 1; SELECT 2", OpSelect, "", "", true, false},
		{"multi-trailing-semicolon", "SELECT 1;", OpSelect, "", "", false, false},
		{"line-comment", "-- hi\nSELECT * FROM t", OpSelect, "", "t", false, false},
		{"block-comment", "/* x */ SELECT * FROM t", OpSelect, "", "t", false, false},
		{"backtick-with-escape", "SELECT * FROM `weird``name`", OpSelect, "", "weird`name", false, false},
		{"unknown", "DO SOMETHING", OpUnknown, "", "", false, false},
		{"empty", "   ", OpUnknown, "", "", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ClassifySQL(c.sql)
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
