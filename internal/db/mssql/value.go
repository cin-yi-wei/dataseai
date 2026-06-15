package mssql

import (
	"database/sql"
	"fmt"
	"strings"
)

func normalizeValue(v any, dbType string) any {
	switch x := v.(type) {
	case []byte:
		if strings.EqualFold(dbType, "uniqueidentifier") && len(x) == 16 {
			return formatUniqueidentifier(x)
		}
		return string(x)
	default:
		return v
	}
}

func normalizeValueForCSV(v any, dbType string) string {
	switch x := normalizeValue(v, dbType).(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func columnDatabaseTypes(cols []*sql.ColumnType) []string {
	types := make([]string, len(cols))
	for i, c := range cols {
		types[i] = c.DatabaseTypeName()
	}
	return types
}

func formatUniqueidentifier(b []byte) string {
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0],
		b[5], b[4],
		b[7], b[6],
		b[8], b[9],
		b[10], b[11], b[12], b[13], b[14], b[15])
}
