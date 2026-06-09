package pg

import "testing"

func TestQuoteIdent(t *testing.T) {
	d := PG{}
	cases := []struct{ in, want string }{
		{"users", `"users"`},
		{"my table", `"my table"`},
		{`weird"name`, `"weird""name"`},
		{"", `""`},
	}
	for _, c := range cases {
		if got := d.QuoteIdent(c.in); got != c.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	d := PG{}
	if got := d.Placeholder(1); got != "$1" {
		t.Fatalf("Placeholder(1) = %q, want $1", got)
	}
	if got := d.Placeholder(42); got != "$42" {
		t.Fatalf("Placeholder(42) = %q, want $42", got)
	}
}
