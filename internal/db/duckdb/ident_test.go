package duckdb

import "testing"

func TestQuoteIdent(t *testing.T) {
	d := DuckDB{}
	tests := []struct{ in, want string }{
		{"foo", `"foo"`},
		{`fo"o`, `"fo""o"`},
		{"", `""`},
	}
	for _, tc := range tests {
		if got := d.QuoteIdent(tc.in); got != tc.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	d := DuckDB{}
	for _, i := range []int{1, 5, 100} {
		if got := d.Placeholder(i); got != "?" {
			t.Errorf("Placeholder(%d) = %q", i, got)
		}
	}
}
