package pg

import (
	"strconv"
	"strings"
)

// QuoteIdent wraps a PG identifier in double quotes. Embedded double quotes
// are escaped by doubling them — same rule as MySQL's backtick scheme.
func (PG) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Placeholder returns the PG-style positional placeholder for index n.
// PG indexes are 1-based; n=1 yields "$1".
func (PG) Placeholder(n int) string {
	return "$" + strconv.Itoa(n)
}
