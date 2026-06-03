package mysql

import "testing"

func TestQuoteIdent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"users", "`users`"},
		{"my table", "`my table`"},
		{"weird`name", "`weird``name`"},
		{"", "``"},
	}
	for _, c := range cases {
		got := QuoteIdent(c.in)
		if got != c.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
