package bytehouse

import "testing"

func TestQuoteIdent(t *testing.T) {
	d := BH{}
	cases := []struct{ in, want string }{
		{"users", "`users`"},
		{"my table", "`my table`"},
		{"weird`name", "`weird``name`"},
		{"", "``"},
	}
	for _, c := range cases {
		if got := d.QuoteIdent(c.in); got != c.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	d := BH{}
	for _, n := range []int{1, 5, 42} {
		if got := d.Placeholder(n); got != "?" {
			t.Errorf("Placeholder(%d) = %q, want ?", n, got)
		}
	}
}
