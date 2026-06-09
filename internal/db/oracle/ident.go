package oracle

import (
	"strconv"
	"strings"
)

// QuoteIdent wraps an Oracle identifier in double-quotes to preserve case.
// Without quoting Oracle uppercases all identifiers.
func (Oracle) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Placeholder returns :N (1-based) — Oracle's positional bind variable syntax.
func (Oracle) Placeholder(i int) string { return ":" + strconv.Itoa(i) }
