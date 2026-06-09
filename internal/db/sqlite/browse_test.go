package sqlite

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

var _ SQLite = SQLite{}

func TestBuildWhereClauseEmpty(t *testing.T) {
	clause, args := buildWhereClause(SQLite{}, nil)
	if clause != "" {
		t.Errorf("expected empty clause, got %q", clause)
	}
	if args != nil {
		t.Errorf("expected nil args, got %v", args)
	}
}

func TestBuildWhereClauseEqual(t *testing.T) {
	filters := []db.Filter{
		{Column: "name", Operator: "=", Value: "alice"},
		{Column: "age", Operator: ">", Value: "18"},
	}
	clause, args := buildWhereClause(SQLite{}, filters)
	want := ` WHERE "name" = ? AND "age" > ?`
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 2 || args[0] != "alice" || args[1] != "18" {
		t.Errorf("args = %v", args)
	}
}

func TestBuildWhereClauseContains(t *testing.T) {
	filters := []db.Filter{{Column: "bio", Operator: "Contains", Value: "golang"}}
	clause, args := buildWhereClause(SQLite{}, filters)
	want := ` WHERE "bio" LIKE ?`
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 1 || args[0] != "%golang%" {
		t.Errorf("args = %v", args)
	}
}

func TestBuildWhereClauseIN(t *testing.T) {
	filters := []db.Filter{{Column: "status", Operator: "IN", Value: "active, pending"}}
	clause, args := buildWhereClause(SQLite{}, filters)
	want := ` WHERE "status" IN (?, ?)`
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 2 || args[0] != "active" || args[1] != "pending" {
		t.Errorf("args = %v", args)
	}
}

func TestBuildWhereClauseBETWEEN(t *testing.T) {
	filters := []db.Filter{{Column: "age", Operator: "BETWEEN", Value: "18, 65"}}
	clause, args := buildWhereClause(SQLite{}, filters)
	want := ` WHERE "age" BETWEEN ? AND ?`
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 2 || args[0] != "18" || args[1] != "65" {
		t.Errorf("args = %v", args)
	}
}
