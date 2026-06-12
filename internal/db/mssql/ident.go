package mssql

import (
	"strconv"
	"strings"
)

// defaultSchema is the schema MSSQL objects are assumed to live under. The
// sidebar surfaces the connected database (DB_NAME()) rather than schemas, so
// callers pass a database name where other dialects pass a schema; objects are
// resolved under dbo, which is the default schema for db_owner-created tables.
const defaultSchema = "dbo"

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
