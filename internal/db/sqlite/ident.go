package sqlite

import "strings"

// QuoteIdent wraps a SQLite identifier in double-quotes, doubling any
// embedded double-quotes to escape them. SQLite also accepts backtick and
// bracket quoting, but double-quote is the SQL standard form.
func (SQLite) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Placeholder returns "?" — SQLite uses positional anonymous placeholders.
// The index argument is ignored.
func (SQLite) Placeholder(int) string { return "?" }
