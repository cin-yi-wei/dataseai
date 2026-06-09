package pg

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

// compile-time: PG satisfies the browse portion of db.Dialect.
// Full interface satisfaction is checked in dialect.go's UnimplementedDialect embedding.
var _ PG = PG{}

func TestBuildPGWhereClauseEmpty(t *testing.T) {
	clause, args := buildPGWhereClause(nil)
	if clause != "" {
		t.Errorf("expected empty clause, got %q", clause)
	}
	if args != nil {
		t.Errorf("expected nil args, got %v", args)
	}

	clause2, args2 := buildPGWhereClause([]db.Filter{})
	if clause2 != "" {
		t.Errorf("expected empty clause for empty slice, got %q", clause2)
	}
	if args2 != nil {
		t.Errorf("expected nil args for empty slice, got %v", args2)
	}
}

func TestBuildPGWhereClauseEqualAndLike(t *testing.T) {
	filters := []db.Filter{
		{Column: "name", Operator: "=", Value: "alice"},
		{Column: "email", Operator: "LIKE", Value: "%@example.com"},
	}

	clause, args := buildPGWhereClause(filters)

	wantClause := ` WHERE "name" = $1 AND "email" LIKE $2`
	if clause != wantClause {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, wantClause)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "alice" {
		t.Errorf("args[0] = %v, want alice", args[0])
	}
	if args[1] != "%@example.com" {
		t.Errorf("args[1] = %v, want %%@example.com", args[1])
	}
}

func TestBuildPGWhereClauseContains(t *testing.T) {
	filters := []db.Filter{
		{Column: "bio", Operator: "Contains", Value: "golang"},
	}
	clause, args := buildPGWhereClause(filters)
	wantClause := ` WHERE "bio" LIKE $1`
	if clause != wantClause {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, wantClause)
	}
	if len(args) != 1 || args[0] != "%golang%" {
		t.Errorf("args = %v, want [%%golang%%]", args)
	}
}

func TestBuildPGWhereClauseNullOps(t *testing.T) {
	filters := []db.Filter{
		{Column: "deleted_at", Operator: "IS NULL"},
	}
	clause, args := buildPGWhereClause(filters)
	wantClause := ` WHERE "deleted_at" IS NULL`
	if clause != wantClause {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, wantClause)
	}
	if len(args) != 0 {
		t.Errorf("expected no args for IS NULL, got %v", args)
	}
}

func TestBuildPGWhereClauseIN(t *testing.T) {
	filters := []db.Filter{
		{Column: "status", Operator: "IN", Value: "active, pending, blocked"},
	}
	clause, args := buildPGWhereClause(filters)
	wantClause := ` WHERE "status" IN ($1, $2, $3)`
	if clause != wantClause {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, wantClause)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
	if args[0] != "active" || args[1] != "pending" || args[2] != "blocked" {
		t.Errorf("args = %v, want [active pending blocked]", args)
	}
}

func TestBuildPGWhereClauseBETWEEN(t *testing.T) {
	filters := []db.Filter{
		{Column: "age", Operator: "BETWEEN", Value: "18, 65"},
	}
	clause, args := buildPGWhereClause(filters)
	wantClause := ` WHERE "age" BETWEEN $1 AND $2`
	if clause != wantClause {
		t.Errorf("clause mismatch\n got:  %q\n want: %q", clause, wantClause)
	}
	if len(args) != 2 || args[0] != "18" || args[1] != "65" {
		t.Errorf("args = %v, want [18 65]", args)
	}
}
