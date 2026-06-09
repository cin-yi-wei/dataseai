package bytehouse

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

// compile-time: BH satisfies the browse portion of db.Dialect.
var _ BH = BH{}

func TestBuildWhereClauseEmpty(t *testing.T) {
	b := BH{}
	clause, args := buildWhereClause(b, nil)
	if clause != "" {
		t.Errorf("expected empty clause, got %q", clause)
	}
	if args != nil {
		t.Errorf("expected nil args, got %v", args)
	}
}

func TestBuildWhereClauseEqualAndLike(t *testing.T) {
	b := BH{}
	filters := []db.Filter{
		{Column: "name", Operator: "=", Value: "alice"},
		{Column: "email", Operator: "LIKE", Value: "%@example.com"},
	}
	clause, args := buildWhereClause(b, filters)
	want := " WHERE `name` = ? AND `email` LIKE ?"
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 2 || args[0] != "alice" || args[1] != "%@example.com" {
		t.Errorf("args = %v", args)
	}
}

func TestBuildWhereClauseContains(t *testing.T) {
	b := BH{}
	filters := []db.Filter{{Column: "bio", Operator: "Contains", Value: "golang"}}
	clause, args := buildWhereClause(b, filters)
	want := " WHERE `bio` LIKE ?"
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 1 || args[0] != "%golang%" {
		t.Errorf("args = %v", args)
	}
}

func TestBuildWhereClauseIN(t *testing.T) {
	b := BH{}
	filters := []db.Filter{{Column: "status", Operator: "IN", Value: "active, pending"}}
	clause, args := buildWhereClause(b, filters)
	want := " WHERE `status` IN (?, ?)"
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 2 || args[0] != "active" || args[1] != "pending" {
		t.Errorf("args = %v", args)
	}
}

func TestBuildWhereClauseBETWEEN(t *testing.T) {
	b := BH{}
	filters := []db.Filter{{Column: "age", Operator: "BETWEEN", Value: "18, 65"}}
	clause, args := buildWhereClause(b, filters)
	want := " WHERE `age` BETWEEN ? AND ?"
	if clause != want {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, want)
	}
	if len(args) != 2 || args[0] != "18" || args[1] != "65" {
		t.Errorf("args = %v", args)
	}
}
