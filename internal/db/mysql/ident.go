package mysql

import "strings"

// QuoteIdent wraps a MySQL identifier in backticks, doubling any embedded
// backticks to escape them. Use for any user-controlled table/column/db name.
func (MySQL) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// Placeholder returns "?" — MySQL uses positional anonymous placeholders.
// The index argument is ignored.
func (MySQL) Placeholder(int) string { return "?" }
