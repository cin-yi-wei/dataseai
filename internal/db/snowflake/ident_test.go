package snowflake

import "testing"

func TestQuoteIdent(t *testing.T) {
	s := Snowflake{}
	tests := []struct{ in, want string }{
		{"foo", `"foo"`},
		{`fo"o`, `"fo""o"`},
		{"My Table", `"My Table"`},
		{"", `""`},
	}
	for _, tc := range tests {
		if got := s.QuoteIdent(tc.in); got != tc.want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	s := Snowflake{}
	for _, i := range []int{1, 5, 100} {
		if got := s.Placeholder(i); got != "?" {
			t.Errorf("Placeholder(%d) = %q", i, got)
		}
	}
}
