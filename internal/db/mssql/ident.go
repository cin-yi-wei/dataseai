package mssql

import (
	"strconv"
	"strings"
)

// QuoteIdent wraps a SQL Server identifier in square brackets.
// Embedded ] are escaped by doubling them.
func (MSSQL) QuoteIdent(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

// Placeholder returns the SQL Server @pN positional placeholder.
// n is 1-based.
func (MSSQL) Placeholder(n int) string {
	return "@p" + strconv.Itoa(n)
}
