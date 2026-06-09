package snowflake

import "strings"

// QuoteIdent wraps a Snowflake identifier in double-quotes preserving case.
// Unquoted Snowflake identifiers are upper-cased by default; double-quoting
// preserves the exact case as stored.
func (Snowflake) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (Snowflake) Placeholder(int) string { return "?" }
