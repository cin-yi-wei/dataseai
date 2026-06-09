package bytehouse

import (
	"strconv"
	"strings"
)

// QuoteIdent wraps a ByteHouse/ClickHouse identifier in backticks.
// Embedded backticks are escaped by doubling them.
func (BH) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// Placeholder returns ? for all positions. ByteHouse uses positional ?
// placeholders (same as MySQL).
func (BH) Placeholder(_ int) string { return "?" }

// placeholderN returns ? repeated n times for use in IN/VALUES lists.
func placeholderN(n int) string {
	if n <= 0 {
		return ""
	}
	phs := make([]string, n)
	for i := range phs {
		phs[i] = "?"
	}
	return strings.Join(phs, ", ")
}

var _ = strconv.Itoa // keep import
