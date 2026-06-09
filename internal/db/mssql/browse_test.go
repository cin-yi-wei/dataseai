package mssql

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

// compile-time: MSSQL satisfies the browse portion of db.Dialect.
var _ MSSQL = MSSQL{}

func TestBuildWhereClauseEmpty(t *testing.T) {
	m := MSSQL{}
	clause, args := buildWhereClause(m, nil)
	if clause != "" {
		t.Errorf("expected empty clause, got %q", clause)
	}
	if args != nil {
		t.Errorf("expected nil args, got %v", args)
	}

	clause2, args2 := buildWhereClause(m, []db.Filter{})
	if clause2 != "" {
		t.Errorf("expected empty clause for empty slice, got %q", clause2)
	}
	if args2 != nil {
		t.Errorf("expected nil args for empty slice, got %v", args2)
	}
}

func TestBuildWhereClauseEqualAndLike(t *testing.T) {
	m := MSSQL{}
	filters := []db.Filter{
		{Column: "name", Operator: "=", Value: "alice"},
		{Column: "email", Operator: "LIKE", Value: "%@example.com"},
	}
	clause, args := buildWhereClause(m, filters)
	want := " WHERE [name] = @p1 AND [email] LIKE @p2"
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 2 || args[0] != "alice" || args[1] != "%@example.com" {
		t.Errorf("args = %v, want [alice %%@example.com]", args)
	}
}

func TestBuildWhereClauseContains(t *testing.T) {
	m := MSSQL{}
	filters := []db.Filter{{Column: "bio", Operator: "Contains", Value: "golang"}}
	clause, args := buildWhereClause(m, filters)
	want := " WHERE [bio] LIKE @p1"
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 1 || args[0] != "%golang%" {
		t.Errorf("args = %v, want [%%golang%%]", args)
	}
}

func TestBuildWhereClauseNullOps(t *testing.T) {
	m := MSSQL{}
	filters := []db.Filter{{Column: "deleted_at", Operator: "IS NULL"}}
	clause, args := buildWhereClause(m, filters)
	want := " WHERE [deleted_at] IS NULL"
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 0 {
		t.Errorf("expected no args for IS NULL, got %v", args)
	}
}

func TestBuildWhereClauseIN(t *testing.T) {
	m := MSSQL{}
	filters := []db.Filter{{Column: "status", Operator: "IN", Value: "active, pending, blocked"}}
	clause, args := buildWhereClause(m, filters)
	want := " WHERE [status] IN (@p1, @p2, @p3)"
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 3 || args[0] != "active" || args[1] != "pending" || args[2] != "blocked" {
		t.Errorf("args = %v", args)
	}
}

func TestBuildWhereClauseBETWEEN(t *testing.T) {
	m := MSSQL{}
	filters := []db.Filter{{Column: "age", Operator: "BETWEEN", Value: "18, 65"}}
	clause, args := buildWhereClause(m, filters)
	want := " WHERE [age] BETWEEN @p1 AND @p2"
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 2 || args[0] != "18" || args[1] != "65" {
		t.Errorf("args = %v, want [18 65]", args)
	}
}
