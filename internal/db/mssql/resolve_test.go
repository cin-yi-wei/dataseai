package mssql

import (
	"errors"
	"testing"
)

func TestParseInvalidObject(t *testing.T) {
	cases := []struct {
		msg  string
		want string
	}{
		{"mssql: Invalid object name 'BS_DataSource'.", "BS_DataSource"},
		{"Invalid object name 'Orders'", "Orders"},
		{"Invalid object name 'dbo.Orders'", ""}, // already qualified -> leave alone
		{"some other error", ""},
		{"", ""},
	}
	for _, c := range cases {
		var err error
		if c.msg != "" {
			err = errors.New(c.msg)
		}
		if got := parseInvalidObject(err); got != c.want {
			t.Errorf("parseInvalidObject(%q) = %q, want %q", c.msg, got, c.want)
		}
	}
}

func TestQualifyName(t *testing.T) {
	loc := tableLoc{DB: "Carbon", Schema: "dbo"}
	cases := []struct {
		in   string
		want string
	}{
		{
			"SELECT * FROM BS_DataSource",
			"SELECT * FROM [Carbon].[dbo].[BS_DataSource]",
		},
		{
			"SELECT * FROM BS_DataSource b WHERE b.id = 1",
			"SELECT * FROM [Carbon].[dbo].[BS_DataSource] b WHERE b.id = 1",
		},
		{
			// case-insensitive match on the bare name
			"select * from bs_datasource",
			"select * from [Carbon].[dbo].[BS_DataSource]",
		},
		{
			// already qualified -> untouched
			"SELECT * FROM other.BS_DataSource",
			"SELECT * FROM other.BS_DataSource",
		},
		{
			// substring of a longer name -> untouched
			"SELECT * FROM BS_DataSourceArchive",
			"SELECT * FROM BS_DataSourceArchive",
		},
	}
	for _, c := range cases {
		if got := qualifyName(c.in, "BS_DataSource", loc); got != c.want {
			t.Errorf("qualifyName(%q)\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}
