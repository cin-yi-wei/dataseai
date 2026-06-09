package clickhouse

import "strings"

// QuoteIdent wraps a ClickHouse identifier in backticks.
func (CH) QuoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func (CH) Placeholder(_ int) string { return "?" }
