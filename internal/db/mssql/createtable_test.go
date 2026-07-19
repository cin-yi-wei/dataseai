package mssql

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestSynthesizeCreateTable_IncludesPKAndFK(t *testing.T) {
	m := MSSQL{}
	cols := []db.Column{
		{Name: "ID", Type: "int", Nullable: false, Key: "PRI"},
		{Name: "CompanyID", Type: "int", Nullable: false, Key: "MUL"},
		{Name: "Name", Type: "nvarchar", Nullable: true, Extra: "max_length=50"},
	}
	pk := []string{"ID"}
	fks := []db.ForeignKey{
		{Name: "FK_x_company", Column: "CompanyID", RefTable: "Company", RefColumn: "ID"},
	}
	sql := synthesizeCreateTable(m, "Activity", cols, pk, fks)

	for _, want := range []string{
		"CREATE TABLE [dbo].[Activity] (",
		"[Name] nvarchar(50) NULL",
		"PRIMARY KEY ([ID])",
		"CONSTRAINT [FK_x_company] FOREIGN KEY ([CompanyID]) REFERENCES [dbo].[Company] ([ID])",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %q in:\n%s", want, sql)
		}
	}
}

func TestSynthesizeCreateTable_CompositeFK(t *testing.T) {
	m := MSSQL{}
	cols := []db.Column{{Name: "A", Type: "int"}, {Name: "B", Type: "int"}}
	fks := []db.ForeignKey{
		{Name: "FK_ab", Column: "A", RefTable: "T", RefColumn: "X"},
		{Name: "FK_ab", Column: "B", RefTable: "T", RefColumn: "Y"},
	}
	sql := synthesizeCreateTable(m, "Tbl", cols, nil, fks)
	if !strings.Contains(sql, "FOREIGN KEY ([A], [B]) REFERENCES [dbo].[T] ([X], [Y])") {
		t.Fatalf("composite FK not grouped:\n%s", sql)
	}
}
