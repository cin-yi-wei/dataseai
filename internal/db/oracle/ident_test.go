package oracle

import "testing"

func TestQuoteIdent(t *testing.T) {
	o := Oracle{}
	tests := []struct{ in, want string }{
		{"foo", `"foo"`},
		{`fo"o`, `"fo""o"`},
		{"My Table", `"My Table"`},
		{"", `""`},
	}
	for _, tc := range tests {
		if got := o.QuoteIdent(tc.in); got != tc.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	o := Oracle{}
	cases := []struct {
		i    int
		want string
	}{
		{1, ":1"}, {2, ":2"}, {10, ":10"},
	}
	for _, tc := range cases {
		if got := o.Placeholder(tc.i); got != tc.want {
			t.Errorf("Placeholder(%d) = %q, want %q", tc.i, got, tc.want)
		}
	}
}
