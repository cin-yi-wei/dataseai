package duckdb

import "strings"

func (DuckDB) QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (DuckDB) Placeholder(int) string { return "?" }
