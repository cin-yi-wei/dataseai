package mysql

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		sql  string
		want StatementKind
	}{
		{"SELECT 1", StmtSelect},
		{"  select * from users", StmtSelect},
		{"-- comment\nSELECT 1", StmtSelect},
		{"/* multi-line */\nSELECT 1", StmtSelect},
		{"SHOW DATABASES", StmtSelect},
		{"EXPLAIN SELECT 1", StmtSelect},
		{"DESCRIBE users", StmtSelect},
		{"DESC users", StmtSelect},
		{"WITH t AS (SELECT 1) SELECT * FROM t", StmtSelect},
		{"INSERT INTO users VALUES (1)", StmtExec},
		{"UPDATE users SET x=1", StmtExec},
		{"DELETE FROM users", StmtExec},
		{"CREATE TABLE x (id INT)", StmtExec},
		{"ALTER TABLE x ADD y INT", StmtExec},
		{"DROP TABLE x", StmtExec},
		{"REPLACE INTO users VALUES (1)", StmtExec},
		{"TRUNCATE users", StmtExec},
		{"CALL myproc()", StmtExec},
		{"BEGIN", StmtExec},
		{"COMMIT", StmtExec},
		{"", StmtSelect}, // empty defaults to select-ish; handler should reject
	}
	for _, c := range cases {
		got := Classify(c.sql)
		if got != c.want {
			t.Errorf("Classify(%q) = %v, want %v", c.sql, got, c.want)
		}
	}
}
