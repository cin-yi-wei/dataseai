package mssql

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
		{"insert-basic", "INSERT INTO t (a) VALUES (1)", db.OpInsert, "", "t", false, false},
		{"insert-schema", "INSERT INTO [dbo].[t] VALUES(1)", db.OpInsert, "dbo", "t", false, false},
		{"insert-select", "INSERT INTO a SELECT * FROM b", db.OpInsert, "", "a", false, false},
		{"update", "UPDATE t SET a=1", db.OpUpdate, "", "t", false, false},
		{"update-schema", "UPDATE [dbo].[t] SET a=1", db.OpUpdate, "dbo", "t", false, false},
		{"delete", "DELETE FROM t WHERE id=1", db.OpDelete, "", "t", false, false},
		{"delete-schema", "DELETE FROM [dbo].[t]", db.OpDelete, "dbo", "t", false, false},
		{"truncate-bare", "TRUNCATE TABLE t", db.OpTruncate, "", "t", false, false},
		{"alter", "ALTER TABLE t ADD x INT", db.OpDDL, "", "t", false, false},
		{"create-forbidden", "CREATE TABLE t (id INT)", db.OpForbidden, "", "", false, false},
		{"drop-forbidden", "DROP TABLE t", db.OpForbidden, "", "", false, false},
		{"grant-forbidden", "GRANT SELECT ON t TO u", db.OpForbidden, "", "", false, false},
		{"revoke-forbidden", "REVOKE SELECT ON t FROM u", db.OpForbidden, "", "", false, false},
		{"deny-forbidden", "DENY SELECT ON t TO u", db.OpForbidden, "", "", false, false},
		{"exec-readmeta", "EXEC sp_help 'dbo.t'", db.OpReadMeta, "", "", false, false},
		{"use-readmeta", "USE mydb", db.OpReadMeta, "", "", false, false},
		{"multi", "SELECT 1; SELECT 2", db.OpSelect, "", "", true, false},
		{"multi-trailing-semicolon", "SELECT 1;", db.OpSelect, "", "", false, false},
		{"line-comment", "-- hi\nSELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"block-comment", "/* x */ SELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"bracket-ident", "SELECT * FROM [my table]", db.OpSelect, "", "my table", false, false},
		{"bracket-schema-table", "SELECT * FROM [dbo].[users]", db.OpSelect, "dbo", "users", false, false},
		{"bracket-escaped", "SELECT * FROM [weird]]name]", db.OpSelect, "", "weird]name", false, false},
		{"with-cte", "WITH cte AS (SELECT 1) SELECT * FROM cte", db.OpSelect, "", "", false, false},
		{"unknown", "DO SOMETHING", db.OpUnknown, "", "", false, false},
		{"empty", "   ", db.OpUnknown, "", "", false, true},
	}

	d := MSSQL{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.ClassifySQL(tc.sql)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none (result=%+v)", got)
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
