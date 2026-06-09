package clickhouse

import "testing"

func TestQuoteIdent(t *testing.T) {
	c := CH{}
	tests := []struct{ in, want string }{
		{"foo", "`foo`"},
		{"fo`o", "`fo``o`"},
		{"", "``"},
	}
	for _, tc := range tests {
		if got := c.QuoteIdent(tc.in); got != tc.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	c := CH{}
	for _, i := range []int{1, 5, 100} {
		if got := c.Placeholder(i); got != "?" {
			t.Errorf("Placeholder(%d) = %q", i, got)
		}
	}
}
