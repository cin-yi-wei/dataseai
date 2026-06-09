package mssql

import "testing"

func TestQuoteIdent(t *testing.T) {
	d := MSSQL{}
	cases := []struct{ in, want string }{
		{"users", "[users]"},
		{"my table", "[my table]"},
		{"weird]name", "[weird]]name]"},
		{"", "[]"},
	}
	for _, c := range cases {
		if got := d.QuoteIdent(c.in); got != c.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	d := MSSQL{}
	if got := d.Placeholder(1); got != "@p1" {
		t.Fatalf("Placeholder(1) = %q, want @p1", got)
	}
	if got := d.Placeholder(42); got != "@p42" {
		t.Fatalf("Placeholder(42) = %q, want @p42", got)
	}
}
