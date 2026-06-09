package pg

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
		// Carryover cases from MySQL (PG-compatible syntax).
		{"select", "SELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"select-no-from", "SELECT 1", db.OpSelect, "", "", false, false},
		{"insert-basic", "INSERT INTO t (a) VALUES (1)", db.OpInsert, "", "t", false, false},
		{"insert-quoted", `INSERT INTO "db"."t" VALUES(1)`, db.OpInsert, "db", "t", false, false},
		{"insert-select", "INSERT INTO a SELECT * FROM b", db.OpInsert, "", "a", false, false},
		{"update", "UPDATE t SET a=1", db.OpUpdate, "", "t", false, false},
		{"update-schema", "UPDATE db.t SET a=1", db.OpUpdate, "db", "t", false, false},
		{"update-join-using", "UPDATE a SET y=b.y FROM b WHERE a.x=b.x", db.OpUpdate, "", "a", false, false},
		{"update-comma", "UPDATE a SET y=1 WHERE x IN (SELECT x FROM b)", db.OpUpdate, "", "a", false, false},
		{"delete", "DELETE FROM t WHERE id=1", db.OpDelete, "", "t", false, false},
		{"delete-quoted", `DELETE FROM "db"."t"`, db.OpDelete, "db", "t", false, false},
		{"truncate-bare", "TRUNCATE t", db.OpTruncate, "", "t", false, false},
		{"truncate-table", "TRUNCATE TABLE db.t", db.OpTruncate, "db", "t", false, false},
		{"alter", "ALTER TABLE t ADD COLUMN x INT", db.OpDDL, "", "t", false, false},
		{"alter-only", "ALTER TABLE ONLY t ADD COLUMN x INT", db.OpDDL, "", "t", false, false},
		{"rename-table", "RENAME TABLE a TO b", db.OpDDL, "", "a", false, false},
		{"create-forbidden", "CREATE TABLE t (id INT)", db.OpForbidden, "", "", false, false},
		{"drop-forbidden", "DROP TABLE t", db.OpForbidden, "", "", false, false},
		{"grant-forbidden", "GRANT SELECT ON t TO u", db.OpForbidden, "", "", false, false},
		{"revoke-forbidden", "REVOKE SELECT ON t FROM u", db.OpForbidden, "", "", false, false},
		{"show", "SHOW search_path", db.OpReadMeta, "", "", false, false},
		{"explain", "EXPLAIN SELECT * FROM t", db.OpReadMeta, "", "", false, false},
		{"multi", "SELECT 1; SELECT 2", db.OpSelect, "", "", true, false},
		{"multi-trailing-semicolon", "SELECT 1;", db.OpSelect, "", "", false, false},
		{"line-comment", "-- hi\nSELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"block-comment", "/* x */ SELECT * FROM t", db.OpSelect, "", "t", false, false},
		{"double-quote-with-escape", `SELECT * FROM "weird""name"`, db.OpSelect, "", `weird"name`, false, false},
		{"unknown", "DO SOMETHING", db.OpUnknown, "", "", false, false},
		{"empty", "   ", db.OpUnknown, "", "", false, true},

		// PG-specific cases.
		{"select_with_recursive", `WITH RECURSIVE t AS (SELECT 1) SELECT * FROM t`, db.OpSelect, "", "", false, false},
		{"select_with_plain", `WITH t AS (SELECT 1) SELECT * FROM t`, db.OpSelect, "", "", false, false},
		{"delete_only", `DELETE FROM ONLY mytable WHERE id=1`, db.OpDelete, "", "mytable", false, false},
		{"delete_only_no_from", `DELETE ONLY mytable WHERE id=1`, db.OpDelete, "", "mytable", false, false},
		{"update_only", `UPDATE ONLY mytable SET a=1`, db.OpUpdate, "", "mytable", false, false},
		{"insert_returning", `INSERT INTO users (name) VALUES ('x') RETURNING id`, db.OpInsert, "", "users", false, false},
		{"update_returning", `UPDATE users SET name='x' WHERE id=1 RETURNING id`, db.OpUpdate, "", "users", false, false},
		{"delete_returning", `DELETE FROM users WHERE id=1 RETURNING id`, db.OpDelete, "", "users", false, false},
		{"schema_qualified", `SELECT * FROM "myschema"."mytable"`, db.OpSelect, "myschema", "mytable", false, false},
		{"forbidden_copy", `COPY foo FROM '/tmp/x.csv'`, db.OpForbidden, "", "", false, false},
		{"forbidden_listen", `LISTEN myevent`, db.OpForbidden, "", "", false, false},
		{"forbidden_notify", `NOTIFY myevent, 'payload'`, db.OpForbidden, "", "", false, false},
		{"forbidden_unlisten", `UNLISTEN myevent`, db.OpForbidden, "", "", false, false},
		{"forbidden_vacuum", `VACUUM ANALYZE t`, db.OpForbidden, "", "", false, false},
		{"forbidden_analyze", `ANALYZE t`, db.OpForbidden, "", "", false, false},
		{"forbidden_reindex", `REINDEX TABLE t`, db.OpForbidden, "", "", false, false},
		{"forbidden_cluster", `CLUSTER t USING t_pkey`, db.OpForbidden, "", "", false, false},
		{"forbidden_security_label", `SECURITY LABEL ON TABLE t IS 'x'`, db.OpForbidden, "", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := PG{}.ClassifySQL(c.sql)
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
