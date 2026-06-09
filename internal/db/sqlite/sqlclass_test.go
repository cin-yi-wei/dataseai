package sqlite

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
		{"insert", `INSERT INTO "users" (id) VALUES (1)`, db.OpInsert, "", "users", false, false},
		{"insert-schema", `INSERT INTO "mydb"."t" VALUES(1)`, db.OpInsert, "mydb", "t", false, false},
		{"insert-backtick", "INSERT INTO `db`.`t` VALUES(1)", db.OpInsert, "db", "t", false, false},
		{"update", `UPDATE "t" SET a=1`, db.OpUpdate, "", "t", false, false},
		{"update-bare", "UPDATE t SET a=1", db.OpUpdate, "", "t", false, false},
		{"delete", `DELETE FROM "t" WHERE id=1`, db.OpDelete, "", "t", false, false},
		{"alter", `ALTER TABLE "t" ADD COLUMN x TEXT`, db.OpDDL, "", "t", false, false},
		{"create-forbidden", "CREATE TABLE t (id INTEGER PRIMARY KEY)", db.OpForbidden, "", "", false, false},
		{"drop-forbidden", "DROP TABLE t", db.OpForbidden, "", "", false, false},
		{"attach-forbidden", "ATTACH DATABASE 'x.db' AS x", db.OpForbidden, "", "", false, false},
		{"pragma-readmeta", "PRAGMA table_info(users)", db.OpReadMeta, "", "", false, false},
		{"explain-readmeta", "EXPLAIN SELECT * FROM t", db.OpReadMeta, "", "", false, false},
		{"multi", "SELECT 1; SELECT 2", db.OpSelect, "", "", true, false},
		{"multi-trailing", "SELECT 1;", db.OpSelect, "", "", false, false},
		{"line-comment", "-- hi\nSELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"block-comment", "/* x */ SELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"double-quote-schema", `SELECT * FROM "mydb"."users"`, db.OpSelect, "mydb", "users", false, false},
		{"bracket-ident", "SELECT * FROM [mydb].[users]", db.OpSelect, "mydb", "users", false, false},
		{"with-cte", "WITH cte AS (SELECT 1) SELECT * FROM cte", db.OpSelect, "", "", false, false},
		{"empty", "   ", db.OpUnknown, "", "", false, true},
	}

	d := SQLite{}
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
